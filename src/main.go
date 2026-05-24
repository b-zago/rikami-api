package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type EnvEntry struct {
	EnvName string
	EnvVals map[string]string
}

type RequestSummon struct {
	Vessel     string
	LibVersion string
	Name       string
	Envs       []EnvEntry
}

func Check(err error) {
	if err != nil {
		panic(err)
	}
}

func CheckPrint(err error) {
	if err != nil {
		fmt.Println(err)
	}
}

func verifyHMAC(body []byte, authHeader string, token string) bool {
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "HMAC" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write(body)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(parts[1]))
}

func handleSummon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		http.Error(w, "missing user id", http.StatusBadRequest)
		return
	}

	creds := readEnvs(userID)
	if creds["user"] == "" || creds["token"] == "" {
		http.Error(w, "unknown user", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	if !verifyHMAC(body, r.Header.Get("Authorization"), creds["token"]) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req RequestSummon
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// fmt.Printf("User:       %s\n", creds["user"])
	// fmt.Printf("Vessel:     %s\n", req.Vessel)
	// fmt.Printf("LibVersion: %s\n", req.LibVersion)
	// fmt.Printf("Name:       %s\n", req.Name)
	// for _, env := range req.Envs {
	// 	fmt.Printf("Env: %s\n", env.EnvName)
	// 	for k, v := range env.EnvVals {
	// 		fmt.Printf("  %s=%s\n", k, v)
	// 	}
	// }

	data, err := json.Marshal(TargetRepoSummon(&req, creds["user"]))
	if err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func readEnvs(userID string) map[string]string {
	return map[string]string{
		"user":  os.Getenv("USER_" + userID),
		"token": os.Getenv("TOKEN_" + userID),
	}
}

func setEnvs() {
	os.Setenv("SEALED_SECRETS_CERT", "/app/cert.pem")
}

func main() {
	fmt.Println("Starting Rikami controller...")
	setEnvs()
	RepoSync()
	TargetRepoSync()
	http.HandleFunc("/summon", handleSummon)
	fmt.Println("listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("server error:", err)
	}
}
