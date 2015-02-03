package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mitchellh/go-homedir"
	flag "github.com/ogier/pflag"
)

var Path *string
var Buffer []string
var Results []string
var Verbose *bool
var ExpectedRepoOwner *string

type Repo struct {
	path     string
	modified int
	added    int
	deleted  int
	synced   bool
}

type RepoList struct {
	count int
	repo  []Repo
}

func (r *Repo) Print() {
	fmt.Printf("path:%s; modified:%d; added:%d; deleted%d; synced:%v", r.path, r.modified, r.added, r.deleted, r.synced)
}

func (r *RepoList) Print() {
	for _, repo := range r.repo {
		repo.Print()
	}
}

func main() {
	HandleCommandlineArgs()
	FindGitRepos(*Path)
	if len(Buffer) > 0 {
		for _, filepath := range Buffer {
			InspectGitRepos(filepath, *ExpectedRepoOwner)
		}
	}
	ResultsToStruct()
}

func ResultsToStruct(rep RepoList) {
	var mod, add, del int
	var sync bool = false
	if len(Results) > 0 {
		for _, result := range Results {
			mod, add, del = analyzeRepoStatus(result)
			if mod == 0 && add == 0 && del == 0 {
				sync = true
			}
			Output = append(Output, output{modified: mod, added: add, deleted: del, synced: sync})

			mod = 0
			add = 0
			del = 0
			sync = false
		}
	}
}

func HandleCommandlineArgs() error {
	home, err := homedir.Dir()
	if err != nil {
		log.Fatal(err)
	}
	Path = flag.StringP("path", "p", home, "Directory to analyze")
	*Path, err = homedir.Expand(*Path)
	if err != nil {
		log.Fatal(err)
	}
	ExpectedRepoOwner = flag.StringP("owner", "o", "", "Owner username of the repository")
	Verbose = flag.BoolP("verbose", "v", false, "Print supplementary information")
	flag.Parse()

	if *ExpectedRepoOwner == "" {
		log.Fatal(errors.New("Missing owner argument (-o)"))
		os.Exit(-1)
	}

	return nil
}

func WriteOut(filePath string, fileName string) {
	f, err := os.Create(filepath.Join(filePath, fileName))
	if err != nil {
		panic(err)
	}
	defer f.Close()
}

func FindGitRepos(d string) error {
	filepath.Walk(d, WalkGitRepos)
	return nil
}

func WalkGitRepos(filePath string, fileInfo os.FileInfo, err error) error {
	if err != nil {
		fmt.Println(err) // can't walk here,
		return nil       // but continue walking elsewhere
	}
	if fileInfo.IsDir() && fileInfo.Name() == ".git" {
		if *Verbose {
			fmt.Println("Found repo: ", filePath)
		}
		Buffer = append(Buffer, filePath)
		return nil
	}
	return nil
}

func InspectGitRepos(filePath string, repoOwner string) error {
	var search string
	var match bool

	gitConfigData, err := ioutil.ReadFile(filepath.Join(filePath, "config"))
	if err != nil {
		panic(err)
	}
	search = fmt.Sprintf("%s/%s", "url( *)?=( *)?https?://github.com", repoOwner)
	re := regexp.MustCompile(search)
	match = re.MatchString(string(gitConfigData))

	if match {
		Results = append(Results, filePath)
	}
	return nil
}

func analyzeRepoStatus(filePath string) (mod int, add int, del int) {
	reMod := regexp.MustCompile(` M|U \w*`)
	reAdd := regexp.MustCompile(`\?\? \w*`)
	reDel := regexp.MustCompile(` D \w*`)

	filePath = strings.TrimRight(filePath, "/.git")

	os.Chdir(filePath)
	results, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		fmt.Println(err)
		return
	}

	tmp := ""
	for _, result := range results {
		tmp += string(result)
	}

	mod = len(reMod.FindAllStringSubmatch(tmp, -1))
	add = len(reAdd.FindAllStringSubmatch(tmp, -1))
	del = len(reDel.FindAllStringSubmatch(tmp, -1))

	return mod, add, del
}
