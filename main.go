package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"regexp"
	"runtime"
	"strings"
	"time"

	markdown "github.com/MichaelMure/go-term-markdown"
	"github.com/fatih/color"
	"github.com/go-git/go-git/v5"
	openai "github.com/sashabaranov/go-openai"
	"github.com/urfave/cli/v2"
)

var (
	pagePath = "wiki_pages"
	wikiGit  = "git@github.com:u2takey/gptwiki-pages.git"
)

func main() {
	markdown.BlueBgItalic = color.New(0, color.Italic).SprintFunc()
	rand.Seed(time.Now().Unix())

	app := cli.App{
		Name:                 "wiki",
		EnableBashCompletion: true,
		Description:          `https://github.com/u2takey/gptwiki`,
		Commands: []*cli.Command{
			Init(),
			Update(),
		},
		Flags: []cli.Flag{},

		Action: func(c *cli.Context) error {
			config, err := ReadConfig()
			if err != nil {
				fmt.Println("read config failed, please call init first")
				return err
			}

			args := c.Args().Slice()
			if len(args) == 0 {
				fmt.Println("please input wiki pages you want to query")
				return nil
			}

			pages := "pages"
			if len(config.Lang) > 0 {
				pages += "." + config.Lang
			}

			commandString := strings.Join(args, " ")
			commandPath, err := WikiPath(path.Join(GetWikiPath(), pages), commandString)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			if commandPath == "" {
				fmt.Println("command page not found, querying from chatgpt...")
				data, err := Query(config.GptKey, commandString, config.Prompt)
				if err != nil {
					return err
				}
				fmt.Printf("\n\n")
				render(string(data))
				fmt.Printf("\n\n")
				fmt.Printf("would you want to save this wiki to upstream? [y/n]: ")
				reader := bufio.NewReader(os.Stdin)
				line, err := reader.ReadString('\n')
				if err != nil {
					return err
				}
				if strings.ToLower(strings.TrimSpace(line)) == "n" {
					return nil
				}
				fmt.Printf("please set folder you want to set. [default: common; available: linux, osx ..]: ")
				line, err = reader.ReadString('\n')
				if err != nil {
					return err
				}
				if strings.ToLower(strings.TrimSpace(line)) != "" {
					config.Folder = strings.ToLower(strings.TrimSpace(line))
				}
				return SavePage("chatgpt", pages, config.Folder, commandString, data, config.UserName)
			} else {
				data, err := ioutil.ReadFile(commandPath)
				if err != nil {
					return err
				}

				render(string(data))
			}
			return nil
		},
		Version: "0.1",
	}

	_ = app.Run(os.Args)
	//if err != nil {
	//	log.Fatalln(err)
	//}
}

var codeLine = regexp.MustCompile("(?m)^`(.{2,})`$")

func render(text string) {
	s := codeLine.ReplaceAllString(text, "\n```\n$1\n```\n")
	result := markdown.Render(s, 120, 2)
	fmt.Println(string(result))
}

func SavePage(source, pages, folder, command, wiki, branch string) error {
	wikiPath := GetWikiPath()
	if exist := CheckPathExist(wikiPath); !exist {
		fmt.Println("not initiated, please call init first")
		return errors.New("not initiated")
	}
	//r, err := git.PlainOpen(wikiPath)
	//if err != nil {
	//	fmt.Println("open wiki repo failed", err)
	//	return err
	//}
	// Get the working directory for the repository
	//w, err := r.Worktree()
	//if err != nil {
	//	fmt.Println("open wiki folder failed", err)
	//	return err
	//}
	commandPath := GetWikiPathForCommand(pages, folder, command)
	if err := ioutil.WriteFile(commandPath,
		[]byte(fmt.Sprintf("#%s \n## chatgpt \n%s", command, wiki)), 0o666); err != nil {
		return err
	}
	//err = w.AddGlob("*")
	//if err != nil {
	//	fmt.Println("add file failed", err)
	//	return err
	//}
	//
	//_, err = w.Commit(
	//	fmt.Sprintf("add page [%s], source: [%s]", command, source),
	//	&git.CommitOptions{
	//		All:               false,
	//		AllowEmptyCommits: false,
	//	})
	//if err != nil {
	//	fmt.Println("commit changed files failed", err)
	//	return err
	//}
	//err = r.PushContext(context.TODO(), &git.PushOptions{
	//	RemoteName: "origin",
	//	RefSpecs:   []config.RefSpec{config.RefSpec("+main" + ":" + branch)},
	//	Force:      true,
	//})
	//if err != nil {
	//	fmt.Println("push changed files failed", err)
	//	return err
	//}
	e := exec.Command("sh", "-c", fmt.Sprintf("cd %s && git add * && git commit -a -m '%s' && git push origin main:%s && cd -",
		path.Join(GetWikiPath(), pages, folder), fmt.Sprintf("add page [%s], source: [%s]", command, source), branch))
	e.Stdout = os.Stdout
	err := e.Run()
	if err != nil {
		fmt.Println("push changed files failed")
		return err
	}
	return OpenInBrowser(fmt.Sprintf("https://github.com/u2takey/gptwiki-pages/pull/new/%s", branch))
}

func OpenInBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		return fmt.Errorf("unsupported platform")
	}
	cmd.Env = os.Environ()
	return cmd.Start()
}

func Query(key, command, prompt string) (string, error) {
	client := openai.NewClient(key)
	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model:       openai.GPT3Dot5Turbo,
			MaxTokens:   2000,
			Temperature: 1,
			TopP:        1,
			Messages: []openai.ChatCompletionMessage{{
				Role:    openai.ChatMessageRoleUser,
				Content: fmt.Sprintf(prompt, command),
			},
			},
		},
	)

	if err != nil {
		fmt.Println("query command from chatgpt error:", err)
		return "", err
	}
	return resp.Choices[0].Message.Content, nil
}

func Init() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "init wiki pages",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "chatgpt-key"},
			&cli.StringFlag{Name: "folder", Value: "common"},
			&cli.StringFlag{Name: "lang", Value: "zh"},
			&cli.StringFlag{Name: "prompt", Value: "请详细解释命令：%s"},
		},
		Action: func(c *cli.Context) error {
			key := c.String("chatgpt-key")
			folder := c.String("folder")
			lang := c.String("lang")
			prompt := c.String("prompt")
			if len(lang) == 0 {
				prompt = "Please explain the command in detail: %s"
			}
			config := Config{
				GptKey:   key,
				Folder:   folder,
				Lang:     lang,
				Prompt:   prompt,
				UserName: getUserName(),
			}
			err := SaveConfig(config)
			if err != nil {
				return err
			}
			wikiPath := GetWikiPath()
			if exist := CheckPathExist(wikiPath); !exist {
				return DoInit(wikiPath, config.UserName)
			}
			return nil
		},
	}
}

type Config struct {
	GptKey   string
	Folder   string
	Lang     string
	Prompt   string
	UserName string
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz")

func RandStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

var (
	invalidNameCharRE = regexp.MustCompile(`[^a-zA-Z0-9_]`)
)

func getUserName() string {
	return RandStringRunes(8)
}

func SaveConfig(c Config) error {
	data, _ := json.Marshal(c)
	if err := ioutil.WriteFile(GetConfigPath(), []byte(data), 0o666); err != nil {
		fmt.Println("save chatgpt-key failed:", err)
		return err
	}
	return nil
}

func ReadConfig() (*Config, error) {
	if !CheckPathExist(GetConfigPath()) {
		fmt.Println("chatgpt-key not exist, please init with --chatgpt-key=<your key>")
		return nil, os.ErrNotExist
	}
	data, err := ioutil.ReadFile(GetConfigPath())
	if err != nil {
		return nil, err
	}
	config := &Config{}
	return config, json.Unmarshal(data, config)
}

func DoInit(path, userName string) error {
	_, err := git.PlainClone(path, false, &git.CloneOptions{
		URL:      wikiGit,
		Progress: os.Stdout,
	})
	if err != nil {
		fmt.Println("clone wiki repo failed", err)
		return err
	}

	fmt.Println("init success")
	return nil
}

func GetWikiPath() string {
	h, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return path.Join(h, pagePath)
}

func GetConfigPath() string {
	h, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return path.Join(h, ".wiki_config.json")
}

func GetWikiPathForCommand(pages, folder, command string) string {
	return path.Join(GetWikiPath(), pages, folder, command+".md")
}

func readDir(root string) ([]string, error) {
	var files []string
	fileInfo, err := ioutil.ReadDir(root)
	if err != nil {
		return files, err
	}

	for _, file := range fileInfo {
		files = append(files, file.Name())
	}
	return files, nil
}

func WikiPath(folder, command string) (string, error) {
	dirs, err := readDir(folder)
	if err != nil {
		return "", err
	}
	for _, a := range dirs {
		targetPath := path.Join(folder, a, command+".md")
		if CheckPathExist(targetPath) {
			return targetPath, nil
		}
	}
	return "", os.ErrNotExist
}

func CheckPathExist(wikiPath string) bool {
	if _, err := os.Stat(wikiPath); errors.Is(err, os.ErrNotExist) {
		return false
	} else if err != nil {
		panic(err)
	}
	return true
}

func Update() *cli.Command {
	return &cli.Command{
		Name:  "update",
		Usage: "update wiki pages",
		Flags: []cli.Flag{},
		Action: func(c *cli.Context) error {
			wikiPath := GetWikiPath()
			if exist := CheckPathExist(wikiPath); !exist {
				fmt.Println("not initiated, please call init first")
				return nil
			}
			r, err := git.PlainOpen(wikiPath)
			if err != nil {
				fmt.Println("open wiki repo failed", err)
				return err
			}
			w, err := r.Worktree()
			if err != nil {
				fmt.Println("open worktree failed", err)
				return err
			}
			head, err := r.Head()
			if err != nil {
				fmt.Println("get head failed", err)
				return err
			}
			if head.Name() != "refs/heads/main" {
				err = w.Checkout(&git.CheckoutOptions{
					Branch: "refs/heads/main",
				})
				if err != nil {
					fmt.Println("checkout main failed", err)
					return err
				}
			}
			err = w.Pull(&git.PullOptions{RemoteName: "origin"})
			if err != nil {
				if err == git.NoErrAlreadyUpToDate {
					fmt.Println("already uptodate")
					return nil
				} else {
					fmt.Println("update wiki pages failed", err)
					return err
				}
			}

			ref, err := r.Head()
			if err != nil {
				return err
			}
			commit, err := r.CommitObject(ref.Hash())
			if err != nil {
				return err
			}
			fmt.Println("update success, last commit:\n\n", commit.String())
			return nil
		},
	}
}
