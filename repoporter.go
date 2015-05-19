package main

// TODO: Make the font size for conky output adjustable through argument or template
// TODO: Add an option to kill daemon (communicate with it ?)

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
	"time"

	"github.com/mitchellh/go-homedir"
	flag "github.com/ogier/pflag"
)

var (
	PathAnalyzed      *string
	DaemonMode        *bool
	DaemonTimer       *int
	gitFolders        []string
	Verbose           *bool
	pathWriteAsFile   *string
	ExpectedRepoOwner *string
	ConkyOutputPath   *string
	Repos             []Repo
)

type Repo struct {
	path     string
	modified int
	added    int
	deleted  int
	synced   bool
}

func (r *Repo) Print() {
	fmt.Printf("%s; modified:%d; added:%d; deleted%d; synced:%v\n", r.path, r.modified, r.added, r.deleted, r.synced)
}

// TODO: Use it
type RepoList struct {
	repos []Repo
}

// TODO: use it
func (rl *RepoList) Print() {
	for _, r := range rl.repos {
		r.Print()
	}
}

func main() {
	readCommandlineArgs()
	if *DaemonMode == true {
		for {
			gitFolders = nil
			Process()
			// Run conky output at a frequency
			if *ConkyOutputPath != "" {
				ConkyOutput(Repos, *ConkyOutputPath)
			}
			time.Sleep(time.Duration(*DaemonTimer) * time.Second)
		}
	} else if *ConkyOutputPath != "" {
		// Run conky output once
		Process()
		ConkyOutput(Repos, *ConkyOutputPath)
	} else {
		// Output to console
		Process()
		for _, r := range Repos {
			r.Print()
		}
	}
}

func Process() error {
	var results []string
	findGitRepos(*PathAnalyzed)
	if len(gitFolders) > 0 {
		for _, filepath := range gitFolders {
			gitRepoMatchesUser(filepath, *ExpectedRepoOwner)
			if match, _ := gitRepoMatchesUser(filepath, *ExpectedRepoOwner); match == true {
				results = append(results, filepath)
			}
		}
	}
	Repos = make([]Repo, 1)
	Repos = resultsToStruct(results)
	if *Verbose {
		for _, repo := range Repos {
			repo.Print()
		}
	}
	if *pathWriteAsFile != "" {
		WriteAsFile(Repos, *pathWriteAsFile)
	}
	return nil
}

// resultsToStruct converts the repo list to struct
func resultsToStruct(results []string) (repos []Repo) {
	var mod, add, del int
	var sync bool = false
	if len(results) > 0 {
		for _, result := range results {
			mod, add, del = analyzeRepoStatus(result)
			if mod == 0 && add == 0 && del == 0 {
				sync = true
			}
			pathNoTrailingGit := result[0 : len(result)-5]
			repos = append(repos, Repo{path: pathNoTrailingGit, modified: mod, added: add, deleted: del, synced: sync})

			mod = 0
			add = 0
			del = 0
			sync = false
		}
	}
	return repos
}

func readCommandlineArgs() error {
	home, err := homedir.Dir()
	if err != nil {
		log.Fatal(err)
	}
	PathAnalyzed = flag.StringP("path", "p", home, "Directory to analyze")
	*PathAnalyzed, err = homedir.Expand(*PathAnalyzed)
	if err != nil {
		log.Fatal(err)
	}
	ExpectedRepoOwner = flag.StringP("owner", "o", "", "Owner username of the repository")
	Verbose = flag.BoolP("verbose", "v", false, "Print supplementary information")
	DaemonMode = flag.BoolP("daemon", "d", false, "Run as a daemon")
	DaemonTimer = flag.IntP("timer", "t", 10, "Update interval (seconds) when ran as a daemon")
	pathWriteAsFile = flag.StringP("file", "f", "", "Output as file")
	ConkyOutputPath = flag.StringP("conky", "c", "", "Location of conky template")
	flag.Parse()

	if *ExpectedRepoOwner == "" {
		log.Fatal(errors.New("Missing owner argument (-o)"))
		os.Exit(-1)
	}

	return nil
}

// WriteAsFile outputs the struct of git repositories captured that match the username
// selected and outputs it as a file
func WriteAsFile(repos []Repo, filePath string) {
	separator := ","

	f, err := os.Create(filePath)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	f.WriteString(fmt.Sprintf("%s %s %s %s %s %s %s %s %s\n", "path", separator, "added", separator, "deleted",
		separator, "modified", separator, "synced"))
	for _, repo := range repos {
		f.WriteString(fmt.Sprintf("%s %s %v %s %v %s %v %s %v\n", repo.path, separator, repo.added, separator,
			repo.deleted, separator, repo.modified, separator, repo.synced))
	}
}

func ConkyOutput(repos []Repo, filePath string) {
	prefixAdd := "+"
	prefixDel := "-"
	prefixMod := "~"
	separator := " : "

	var err error
	filePath, err = homedir.Expand(filePath)
	if err != nil {
		panic(err)
	}
	data := fmt.Sprintln(
		"#!/bin/bash\n",
		"# \n",
		"# This file is automatically generated\n",
		"# Must have DejaVu sans mono font installed\n",
		"# assumes conky colors:\n",
		"# color 1 : git additions\n",
		"# color 2 : git deltions\n",
		"# color 3 : git modifications\n",
		"# \n")
	for _, r := range repos {
		// initialization
		if r.synced != true {
			data += fmt.Sprintf("echo '${font}")
			if r.added != 0 {
				data += fmt.Sprintf("${color1}%s%v ", prefixAdd, r.added)
			} else {
				data += "   "
			}

			if r.deleted != 0 {
				data += fmt.Sprintf("${color2}%s%v ", prefixDel, r.deleted)
			} else {
				data += "   "
			}

			if r.modified != 0 {
				data += fmt.Sprintf("${color3}%s%v ", prefixMod, r.modified)
			} else {
				data += "   "
			}

			data += fmt.Sprintf("${color}%s${font DejaVu Sans Mono: size=12:style=book}%v", separator, r.path)
			data += fmt.Sprintf("${color}${font}'\n")
		} else {
			fmt.Printf("Skipping the following synced repo: %v\n", r.path)
		}
	}

	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0775)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if f.WriteString(data); err != nil {
		panic(err)
	}
	data = ""
}

func findGitRepos(d string) error {
	filepath.Walk(d, walkGitRepos)
	return nil
}

// WalkGitRepos enters each subdirectory of the selected director, and if it contains ".git" subdirectory, tags it as a git repository
func walkGitRepos(filePath string, fileInfo os.FileInfo, err error) error {
	if err != nil {
		// can't walk here, but continue walking elsewhere
		if *Verbose == true {
			fmt.Println(err)
		}
	}
	if fileInfo.IsDir() && fileInfo.Name() == ".git" {
		if *Verbose {
			fmt.Println("Found repo : ", filePath)
		}
		gitFolders = append(gitFolders, filePath)
	}
	return nil
}

// gitRepoMatchesUser validates each .git directory to validate the owner of the repo. If it matches the username being checked it adds
func gitRepoMatchesUser(filePath string, repoOwner string) (matched bool, e error) {
	var search string

	gitConfigData, err := ioutil.ReadFile(filepath.Join(filePath, "config"))
	if err != nil {
		fmt.Println("Found .git folder not container a 'config' file:", filePath)
	}
	search = fmt.Sprintf("%s/%s", "url( *)?=( *)?https?://github.com", repoOwner)
	re := regexp.MustCompile(search)
	matched = re.MatchString(string(gitConfigData))
	return matched, nil
}

func analyzeRepoStatus(filePath string) (mod int, add int, del int) {
	reMod := regexp.MustCompile(` M|U \w*`)
	reAdd := regexp.MustCompile(`\?\? \w*`)
	reDel := regexp.MustCompile(` ?D \w*`)

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
