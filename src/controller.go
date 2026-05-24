package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

func run(dest string, args ...string) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dest
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	err := cmd.Run()
	Check(err)
}

func RepoSync() {
	owner := "b-zago"
	repo := "rikami-api"
	branch := "main"
	folder := "resources"
	dest := "./resources"
	token := os.Getenv("GITHUB_TOKEN")

	url := fmt.Sprintf("https://oauth2:%s@github.com/%s/%s", token, owner, repo)

	// already initialized — just checkout origin
	if _, err := os.Stat(dest + "/.git"); err == nil {
		run(dest, "git", "fetch", "--depth=1", "origin", branch)
		run(dest, "git", "reset", "--hard", "origin/"+branch)
		run(dest, "git", "clean", "-fdx")
		return
	}

	// first run — sparse clone
	os.MkdirAll(dest, 0755)
	exec.Command("git", "init", dest).Run()
	run(dest, "git", "config", "user.name", "rikami")
	run(dest, "git", "config", "user.email", "rikami@example.com")
	run(dest, "git", "remote", "add", "origin", url)
	run(dest, "git", "sparse-checkout", "init", "--cone")
	run(dest, "git", "sparse-checkout", "set", folder)
	run(dest, "git", "fetch", "--depth=1", "origin", branch)
	run(dest, "git", "checkout", branch)
}

func TargetRepoSync() {
	owner := "b-zago"
	repo := "k3s-cluster"
	branch := "main"
	dest := "./target"
	token := os.Getenv("GITHUB_TOKEN")

	url := fmt.Sprintf("https://oauth2:%s@github.com/%s/%s", token, owner, repo)
	if _, err := os.Stat(dest + "/.git"); err == nil {
		run(dest, "git", "pull", "origin", branch)
		return
	}

	os.MkdirAll(dest, 0755)
	exec.Command("git", "init", dest).Run()
	run(dest, "git", "config", "user.name", "rikami")
	run(dest, "git", "config", "user.email", "rikami@example.com")
	run(dest, "git", "remote", "add", "origin", url)
	run(dest, "git", "pull", "origin", branch)
}

func TargetRepoSummon(req *RequestSummon, user string) []EnvEntry {
	err := fetchCert()
	Check(err)
	RepoSync()
	TargetRepoSync()

	overrideName(req.Vessel, req.Name)

	workdir := filepath.Join("workdir", req.Name)

	generateEnvs(req.Envs, workdir)

	targetArg := fmt.Sprintf("-target=../../target/charts/%s", req.Name)
	run(workdir, "rikami", "summon", req.Vessel, targetArg, "-local", "-conf=../../conf")

	updateVer(req.Name, req.LibVersion)

	commitPush(req.Name, user)

	return getGeneratedSecrets(workdir)
}

func commitPush(vslName string, user string) {
	workdir := "target"
	commitMsg := fmt.Sprintf(`"%s created. Issued by %s"`, vslName, user)

	run(workdir, "git", "add", "charts")
	run(workdir, "git", "commit", "-m", commitMsg)
	run(workdir, "git", "push")
}

func overrideName(vsl string, vslName string) {
	vesPath := filepath.Join("resources", "resources", "vessels", vsl+".ves")
	overrideStr := fmt.Sprintf(`{{override .Chart.Main "name" "%s"}}`, vslName)
	err := replaceInFile(vesPath, `{{request .Chart.Main "name" "Chart name"}}`, overrideStr)
	CheckPrint(err)
}

func updateVer(chartName string, version string) {
	chartPath := filepath.Join("target", "charts", chartName, "Chart.yaml")

	data, err := os.ReadFile(chartPath)
	CheckPrint(err)

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		panic(err)
	}

	deps := doc["dependencies"].([]any)
	deps[0].(map[string]any)["version"] = version

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf, yaml.Indent(2), yaml.IndentSequence(true))
	CheckPrint(enc.Encode(doc))
	enc.Close()
	Check(os.WriteFile(chartPath, buf.Bytes(), 0644))
}

func generateEnvs(envs []EnvEntry, workdir string) {
	os.MkdirAll(workdir, 0755)

	for _, entry := range envs {
		envFile := filepath.Join(workdir, entry.EnvName)
		f, err := os.Create(envFile)
		CheckPrint(err)
		defer f.Close()
		for k, v := range entry.EnvVals {
			envLine := fmt.Sprintf("%s=%s\n", k, v)
			f.WriteString(envLine)
		}
	}
}

func getGeneratedSecrets(workdir string) []EnvEntry {
	files, _ := filepath.Glob(filepath.Join(workdir, "*.secret.random"))
	if files == nil {
		return nil
	}
	var envSlice []EnvEntry
	for _, p := range files {
		envPath := filepath.Join(p)
		f, err := os.ReadFile(envPath)
		CheckPrint(err)
		newEntry := EnvEntry{
			EnvName: filepath.Base(envPath),
			EnvVals: ParseEnvFile(string(f)),
		}
		envSlice = append(envSlice, newEntry)
	}
	return envSlice
}

func fetchCert() error {
	cmd := exec.Command("kubeseal", "--fetch-cert",
		"--controller-name=sealed-secrets",
		"--controller-namespace=kube-system")
	out, err := os.Create("cert.pem")
	if err != nil {
		return err
	}
	defer out.Close()
	cmd.Stdout = out
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func ParseEnvFile(data string) map[string]string {
	newMap := make(map[string]string)
	for line := range strings.SplitSeq(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		newMap[strings.TrimSpace(key)] = value
	}
	return newMap
}

func replaceInFile(path, old, new string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	replaced := strings.ReplaceAll(string(content), old, new)
	return os.WriteFile(path, []byte(replaced), 0644)
}
