package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const requestTimeout = time.Second * 10

var githubToken string

func main() {
	githubToken = os.Getenv("GITHUB_TOKEN")
	if len(githubToken) == 0 {
		log.Println("Error: GITHUB_TOKEN is not set")
		os.Exit(1)
	}

	mergeableIssues := getMergeableIssues()
	if len(mergeableIssues) == 0 {
		log.Println("No mergeable issues.")
		os.Exit(0)
	}

	for _, i := range mergeableIssues {
		err := merge(i)
		if err != nil {
			panic(err)
		}
	}
}

type label struct {
	Name string `json:"name"`
}

type repo struct {
	CloneUrl string `json:"clone_url"`
}

type head struct {
	Repo repo   `json:"repo"`
	Sha  string `json:"sha"`
}

type issue struct {
	Url    string  `json:"url"`
	Labels []label `json:"labels"`
	Head   head    `json:"head"`
}

type issueList struct {
	Issues []issue
}

type mergeMe struct {
	prUrl    string
	cloneUrl string
	sha      string
}

func getOpenPRs() []issue {
	url := "https://api.github.com/repos/mostynb/github-action-test/pulls"
	ctx, _ := context.WithTimeout(context.Background(), requestTimeout)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		panic(err)
	}
	req.Header.Add("Authorization", "Bearer "+githubToken)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Failed to make request")
		panic(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Println("Request failed:", resp.StatusCode)
		panic(fmt.Sprintf("Request failed: %d", resp.StatusCode))
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	var issues []issue
	err = json.Unmarshal(data, &issues)
	if err != nil {
		panic(err)
	}

	return issues
}

func getMergeableIssues() []mergeMe {
	issues := getOpenPRs()

	var result []mergeMe

	for _, i := range issues {
		for _, l := range i.Labels {
			if l.Name == "merge-me" {

				item := mergeMe{
					prUrl:    i.Url,
					cloneUrl: i.Head.Repo.CloneUrl,
					sha:      i.Head.Sha,
				}

				result = append(result, item)
			}
		}
	}

	return result
}

func maybeQuote(in string) string {
	if strings.Contains(in, " ") {
		return strconv.Quote(in)
	}

	return in
}

func quoteCommand(args ...string) string {
	quotedCmd := maybeQuote(args[0])
	for _, a := range args[1:] {
		quotedCmd += " " + maybeQuote(a)
	}

	return quotedCmd
}

func runCmdWithOutput(args ...string) error {
	err, stdout, stderr := runCmd(args...)
	if stdout.Len() > 0 {
		log.Println(stdout)
	}
	if stderr.Len() > 0 {
		log.Println(stderr)
	}

	return err
}

func runCmd(args ...string) (error, *bytes.Buffer, *bytes.Buffer) {
	var err error
	var stdout, stderr bytes.Buffer

	log.Println("CMD:", quoteCommand(args...))

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	return err, &stdout, &stderr
}

func merge(m mergeMe) error {
	log.Println("MERGE", m)

	var err error
	var stdout, stderr *bytes.Buffer

	err = runCmdWithOutput("git", "fetch", m.cloneUrl, m.sha+":sha_"+m.sha)
	if err != nil {
		log.Println("Failed to fetch:", err)
		return err
	}

	err = runCmdWithOutput("git", "checkout", "master")
	if err != nil {
		log.Println("Failed to checkout master:", err)
		return err
	}

	err, stdout, _ = runCmd("git", "rev-parse", "HEAD")
	if err != nil {
		log.Println("Failed to find initial ref")
		return err
	}
	initial_sha := strings.TrimSpace(stdout.String())

	err = runCmdWithOutput("git", "clean", "-dfx")
	if err != nil {
		log.Println("Failed to clean workspace:", err)
		return err
	}

	err, stdout, stderr = runCmd("git", "merge", m.sha)
	if err != nil {
		msg := fmt.Sprintf("Failed to merge: git merge %s\n%s\n%s",
			m.sha, stdout, stderr)
		m.addComment(msg)
		m.removeLabel()

		log.Println("Failed to merge:", err, stdout, stderr)
		runCmdWithOutput("git", "merge", "--abort")
		runCmdWithOutput("git", "reset", "--hard", initial_sha)
		return err
	}

	log.Println("run hooks...")
	filename := "pretend_hook_output.txt"
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		log.Println("pretend hook failed")
		return err
	} else {
		f.WriteString("hello\n")
		f.Close()
		runCmd("git", "add", filename)
	}

	err = runCmdWithOutput("git", "commit", "--amend", "--no-edit")
	if err != nil {
		log.Println("Failed to amend commit:", err)
		runCmdWithOutput("git", "reset", "--hard", initial_sha)
		return err
	}

	err = runCmdWithOutput("git", "push", "origin", "master")
	if err != nil {
		log.Println("Failed to push to master:", err)
		runCmdWithOutput("git", "reset", "--hard", initial_sha)
		return err
	}

	m.removeLabel()
	m.addComment("Merged into master.")
	m.closePR()

	log.Println()

	return nil
}

type addCommentBody struct {
	body string `json:"body"`
}

func (m *mergeMe) addComment(comment string) error {
	url := m.prUrl + "/comments"
	log.Println("POST:", url)

	b := addCommentBody{body: comment}
	data, err := json.Marshal(b)
	if err != nil {
		log.Println("Failed to marshal comment data")
		return err
	}

	ctx, _ := context.WithTimeout(context.Background(), requestTimeout)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		log.Println("Failed to create request")
		return err
	}
	req.Header.Add("Authorization", "Bearer "+githubToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Failed to make request")
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Println("Request failed:", resp.StatusCode)
		return fmt.Errorf("Request failed: %d", resp.StatusCode)
	}

	return nil
}

func (m *mergeMe) removeLabel() error {
	url := m.prUrl + "/labels/merge-me"
	log.Println("DELETE:", url)

	ctx, _ := context.WithTimeout(context.Background(), requestTimeout)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	req.Header.Add("Authorization", "Bearer "+githubToken)
	if err != nil {
		log.Println("Failed to create request")
		return err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Failed to make request")
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Println("Request failed:", resp.StatusCode)
		return fmt.Errorf("Request failed: %d", resp.StatusCode)
	}

	return nil
}

type closePRBody struct {
	state string `json:"state"`
}

func (m *mergeMe) closePR() error {
	log.Println("PATCH:", m.prUrl)

	b := closePRBody{state: "closed"}
	data, err := json.Marshal(b)
	if err != nil {
		log.Println("Failed to marshal issue state data")
		return err
	}

	ctx, _ := context.WithTimeout(context.Background(), requestTimeout)
	req, err := http.NewRequestWithContext(ctx, "GET", m.prUrl, bytes.NewReader(data))
	if err != nil {
		log.Println("Failed to create request")
		return err
	}
	req.Header.Add("Authorization", "Bearer "+githubToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Failed to make request")
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Println("Request failed:", resp.StatusCode)
		return fmt.Errorf("Request failed: %d", resp.StatusCode)
	}

	return nil
}
