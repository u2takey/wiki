package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/sashabaranov/go-openai"
	"github.com/u2takey/go-utils/workqueue"
	"github.com/urfave/cli/v2"
)

var (
	pagePath    = "wiki_pages"
	tldrDirPath = "tldr"
	wikiGit     = "git@github.com:u2takey/gptwiki-pages.git"
	tldrGit     = "git@github.com:tldr-pages/tldr.git"
)

func main() {
	app := cli.App{
		Name:                 "wiki-tool",
		EnableBashCompletion: true,
		Description:          `https://github.com/u2takey/gptwiki`,
		Flags:                []cli.Flag{},
		Commands: []*cli.Command{
			Init(),
		},
		Version: "0.1",
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatalln(err)
	}
}

func Init() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "init wiki pages",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "chatgpt-key"},
			&cli.StringFlag{Name: "folder", Value: "common"},
			&cli.StringFlag{Name: "lang", Value: "zh"},
			&cli.StringFlag{Name: "prompt", Value: ""},
			&cli.BoolFlag{Name: "override", Value: false},
			&cli.IntFlag{Name: "worker", Value: 10},
		},
		Action: func(c *cli.Context) error {
			override := c.Bool("override")
			pages := "pages"
			key := c.String("chatgpt-key")
			folder := c.String("folder")
			lang := c.String("lang")
			prompt := c.String("prompt")
			if len(lang) > 0 {
				pages += "." + lang
				if lang == "zh" && prompt == "" {
					prompt = "请详细解释命令：%s"
				}
			} else {
				if prompt == "" {
					prompt = "Please explain the command in detail: %s"
				}
			}

			fmt.Println("start", pages, folder)
			wikiPath := GetWikiPath()
			if exist := CheckPathExist(wikiPath); !exist {
				_, err := git.PlainClone(wikiPath, false, &git.CloneOptions{
					URL:      wikiGit,
					Progress: os.Stdout,
				})
				if err != nil {
					fmt.Println("clone repo failed", err)
					return err
				} else {
					fmt.Println("clone success")
				}
			}
			tldrPath := GetTLDRPath()
			if exist := CheckPathExist(tldrPath); !exist {
				_, err := git.PlainClone(tldrPath, false, &git.CloneOptions{
					URL:      tldrGit,
					Depth:    1,
					Progress: os.Stdout,
				})
				if err != nil {
					fmt.Println("clone repo failed", err)
					return err
				} else {
					fmt.Println("clone success")
				}
			}

			if !CheckPathExist(filepath.Join(GetWikiPath(), pages, folder)) {
				err := os.MkdirAll(filepath.Join(GetWikiPath(), pages, folder), 0o777)
				if err != nil {
					panic(err)
				}
			}

			var allPaths, failedPath []string
			var lock sync.Mutex
			filepath.WalkDir(filepath.Join(tldrPath, pages, folder),
				func(path string, d fs.DirEntry, err error) error {
					if d.IsDir() {
						return nil
					}
					allPaths = append(allPaths, path)
					return nil
				})
			var a int64 = 1

			workqueue.ParallelizeUntil(context.TODO(), c.Int("worker"), len(allPaths), func(piece int) {
				path := allPaths[piece]
				atomic.AddInt64(&a, 1)
				log.Println("progress:", float64(a)/float64(len(allPaths))*100)
				data, err := ioutil.ReadFile(path)
				if err != nil {
					log.Println(err)
					lock.Lock()
					failedPath = append(failedPath, path)
					lock.Unlock()
					return
				}
				dataString := string(data)
				title := strings.Split(dataString, "\n")[0]
				dataString = strings.TrimPrefix(dataString, title+"\n")
				titleClean := strings.Trim(title, "# ")
				_, fileName := filepath.Split(path)
				targetPath := filepath.Join(GetWikiPath(), pages, folder, fileName)
				if !override {
					if _, err := os.Stat(targetPath); err == nil {
						return
					}
				}

				client := openai.NewClient(key)
				resp, err := client.CreateChatCompletion(
					context.TODO(),
					openai.ChatCompletionRequest{
						Model:       openai.GPT3Dot5Turbo,
						MaxTokens:   2000,
						Temperature: 1,
						TopP:        1,
						Messages: []openai.ChatCompletionMessage{{
							Role:    openai.ChatMessageRoleUser,
							Content: fmt.Sprintf(prompt, titleClean),
						},
						},
					},
				)

				if err != nil {
					log.Println(err)
					if strings.Contains(err.Error(), " Rate limit") {
						time.Sleep(time.Minute)
					}
					lock.Lock()
					failedPath = append(failedPath, path)
					lock.Unlock()
					return
				}
				if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
					log.Println("chatgpt response is empty")
					lock.Lock()
					failedPath = append(failedPath, path)
					lock.Unlock()
					return
				}
				dataFromGpt := resp.Choices[0].Message.Content
				err = ioutil.WriteFile(targetPath,
					[]byte(fmt.Sprintf("%s \n## chatgpt \n%s \n\n## tldr \n %s",
						title, dataFromGpt, dataString)), 0o666)
				if err != nil {
					log.Println(err)
					lock.Lock()
					failedPath = append(failedPath, path)
					lock.Unlock()
					return
				}
			})

			log.Println("failed path", failedPath)
			return nil
		},
	}
}

func GetWikiPath() string {
	h := os.TempDir()
	return path.Join(h, pagePath)
}

func GetTLDRPath() string {
	h := os.TempDir()
	return path.Join(h, tldrDirPath)
}

func CheckPathExist(wikiPath string) bool {
	if _, err := os.Stat(wikiPath); errors.Is(err, os.ErrNotExist) {
		return false
	} else if err != nil {
		panic(err)
	}
	return true
}
