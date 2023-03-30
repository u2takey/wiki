package main

import (
	"math/rand"
	"testing"
	"time"
)

func TestSavePages(t *testing.T) {
	rand.Seed(time.Now().Unix())
	SavePage("test", "pages.zh", "linux", "test", "test", "zkiuiqiw")
}
