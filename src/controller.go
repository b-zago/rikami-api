package main

import (
	"fmt"
	"os"
	"os/exec"
)

func RepoSync() {
	owner := "b-zago"
	repo := "rikami-controller"
	branch := "main"
	folder := "resources"
	dest := "./resources"
	token := os.Getenv("GITHUB_TOKEN")

	url := fmt.Sprintf("https://oauth2:%s@github.com/%s/%s", token, owner, repo)

	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dest
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		cmd.Run()
	}

	// already initialized — just checkout origin
	if _, err := os.Stat(dest + "/.git"); err == nil {
		run("git", "fetch", "--depth=1", "origin", branch)
		run("git", "checkout", branch)
		return
	}

	// first run — sparse clone
	os.MkdirAll(dest, 0755)
	exec.Command("git", "init", dest).Run()
	run("git", "remote", "add", "origin", url)
	run("git", "sparse-checkout", "init", "--cone")
	run("git", "sparse-checkout", "set", folder)
	run("git", "fetch", "--depth=1", "origin", branch)
	run("git", "checkout", branch)
}
