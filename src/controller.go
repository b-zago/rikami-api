package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

func run(dest string, args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dest
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	err := cmd.Run()
	CheckPrint(err, false)
	return err
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
	branch := EnvConf.TargetBranch
	dest := "./target"
	token := os.Getenv("GITHUB_TOKEN")

	url := fmt.Sprintf("https://oauth2:%s@github.com/%s/%s", token, owner, repo)
	if _, err := os.Stat(dest + "/.git"); err == nil {
		run(dest, "git", "pull", "origin", branch)
		return
	}

	os.MkdirAll(dest, 0755)
	exec.Command("git", "init", "-b", branch, dest).Run()
	run(dest, "git", "config", "user.name", "rikami")
	run(dest, "git", "config", "user.email", "rikami@example.com")
	run(dest, "git", "remote", "add", "origin", url)
	run(dest, "git", "pull", "origin", branch)
}

func TargetRepoSummon(req *SummonRequest, user string) []EnvEntry {
	err := fetchCert()
	CheckPrint(err, false)
	RepoSync()
	TargetRepoSync()

	overrideName(req.Vessel, req.Name)

	workdir := filepath.Join("workdir", req.Name)

	generateEnvs(req.Envs, workdir)

	targetArg := fmt.Sprintf("-target=../../target/charts/%s", req.Name)
	run(workdir, "rika", "summon", req.Vessel, targetArg, "-local", "-conf=../../conf")

	updateVer(req.Name, req.LibVersion)

	commitMsg := fmt.Sprintf(`"%s created. Issued by %s"`, req.Name, user)
	commitPush(commitMsg)

	return getGeneratedSecrets(workdir)
}

func TargetRepoApp(req *AppRequest, user string) Response {
	dest := filepath.Join("target", "charts")
	err := run(dest, "rika", "app", req.Action, req.Pattern, "-p="+req.Param, "-local", "-conf=../../conf")
	if err != nil {
		return Response{Message: err.Error()}
	}

	commitMsg := fmt.Sprintf(`"%s was modified with action %s. Issued by %s"`, req.Pattern, req.Action, user)
	commitPush(commitMsg)
	return Response{Message: "App request executed"}
}

func GetFreshCert() Response {
	fetchCert()

	f, err := os.ReadFile("/app/cert.pem")
	CheckPrint(err, false)

	return Response{Message: string(f)}
}

func commitPush(commitMsg string) {
	workdir := "target"

	run(workdir, "git", "add", "charts")
	run(workdir, "git", "commit", "-m", commitMsg)
	run(workdir, "git", "pull", "--rebase", "origin", "main") // absorb concurrent commits
	run(workdir, "git", "push", "origin", "HEAD:main")
}

func overrideName(vsl string, vslName string) {
	vesPath := filepath.Join("resources", "resources", "vessels", vsl+".ves")
	overrideStr := fmt.Sprintf(`{{override .Chart.Main "name" "%s"}}`, vslName)
	err := replaceInFile(vesPath, `{{request .Chart.Main "name" "Chart name"}}`, overrideStr)
	CheckPrint(err, false)
}

func updateVer(chartName string, version string) {
	chartPath := filepath.Join("target", "charts", chartName, "Chart.yaml")

	data, err := os.ReadFile(chartPath)
	CheckPrint(err, false)

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		panic(err)
	}

	deps := doc["dependencies"].([]any)
	deps[0].(map[string]any)["version"] = version

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf, yaml.Indent(2), yaml.IndentSequence(true))
	CheckPrint(enc.Encode(doc), false)
	enc.Close()
	CheckPrint(os.WriteFile(chartPath, buf.Bytes(), 0644), false)
}

func generateEnvs(envs []EnvEntry, workdir string) {
	os.MkdirAll(workdir, 0755)

	for _, entry := range envs {
		envFile := filepath.Join(workdir, entry.EnvName)
		f, err := os.Create(envFile)
		CheckPrint(err, false)
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
		CheckPrint(err, false)
		newEntry := EnvEntry{
			EnvName: filepath.Base(envPath),
			EnvVals: ParseEnvFile(string(f)),
		}
		envSlice = append(envSlice, newEntry)
	}
	return envSlice
}

func fetchCert() error {
	os.Unsetenv("SEALED_SECRETS_CERT")
	cmd := exec.Command("kubeseal", "--fetch-cert",
		"--controller-name=sealed-secrets",
		"--controller-namespace=kube-system")
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return fmt.Errorf("kubeseal exit %d: %s", ee.ExitCode(), ee.Stderr)
		}
		return fmt.Errorf("kubeseal failed: %w", err)
	}
	if len(out) == 0 {
		return errors.New("kubeseal returned empty cert")
	}
	os.Setenv("SEALED_SECRETS_CERT", "/app/cert.pem")
	return os.WriteFile("/app/cert.pem", out, 0644)
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
