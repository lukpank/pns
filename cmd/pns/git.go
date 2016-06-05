// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type GitRepo struct {
	dir string
	env []string
	ref string
	buf bytes.Buffer
}

func NewGitRepo(dir string) *GitRepo {
	env := []string{"GIT_DIR=" + dir, "HOME=" + os.Getenv("HOME")}
	return &GitRepo{dir: dir, env: env}
}

func gitCheckInstalled() (string, error) {
	b, err := exec.Command("git", "version").Output()
	if err != nil {
		return "", err
	}
	b = bytes.TrimSuffix(b, []byte{'\n'})
	if !bytes.HasPrefix(b, []byte("git version ")) {
		return "", fmt.Errorf("unsupported git: %q", b)
	} else {
		return string(b), nil
	}
}

func (g *GitRepo) Init() error {
	if err := os.Mkdir(g.dir, 0755); err != nil {
		return fmt.Errorf("git: failed to create repository: %v", err)
	}
	cmd := g.command("git", "init", "--bare")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git: failed to create repository: %v: %s", err, g.buf.Bytes())
	}
	return nil
}

func (g *GitRepo) Add(fileName string, data []byte) error {
	_, _, err := g.getHEAD()
	if err != nil {
		return err
	}

	cmd := g.command("git", "hash-object", "-w", "--stdin")
	cmd.Stdin = bytes.NewReader(data)
	blobHash, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git: failed to run hash-object: %v: %s", err, g.buf.Bytes())
	}

	cmd = g.command("git", "update-index", "--add", "--cacheinfo", fmt.Sprintf("100644,%s,%s", bytes.TrimSpace(blobHash), fileName))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git: failed to run update-index: %v: %s", err, g.buf.Bytes())
	}

	return nil
}

const RFC2822 = "Mon, 02 Jan 2006 15:04:05 -0700"

func (g *GitRepo) Commit(msg string, authorDate time.Time) error {
	refName, first, err := g.getHEAD()
	if err != nil {
		return err
	}

	cmd := g.command("git", "write-tree")
	treeHash, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git: failed to run write-tree: %v: %s", err, g.buf.Bytes())
	}

	args := []string{"commit-tree"}
	if !first {
		args = append(args, "-p", refName)
	}
	args = append(args, "-m", msg, string(bytes.TrimSpace(treeHash)))
	cmd = g.command("git", args...)
	if !authorDate.IsZero() {
		cmd.Env = append(cmd.Env, "GIT_AUTHOR_DATE="+authorDate.Format(RFC2822))
	}
	commitHash, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git: failed to run commit-tree: %v: %s", err, g.buf.Bytes())
	}

	return g.updateRef(refName, string(bytes.TrimSpace(commitHash)))
}

func (g *GitRepo) updateRef(refName, hash string) error {
	cmd := g.command("git", "update-ref", refName, hash)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git: failed to run update-ref: %v: %s", err, g.buf.Bytes())
	}
	return nil
}

func (g *GitRepo) command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Env = g.env
	g.buf.Reset()
	cmd.Stderr = &g.buf
	return cmd
}

func (g *GitRepo) getHEAD() (ref string, first bool, err error) {
	if g.ref != "" {
		return g.ref, false, nil
	}

	cmd := g.command("git", "symbolic-ref", "HEAD")
	b, err := cmd.Output()
	if err != nil {
		return "", false, fmt.Errorf("git: failed to get symbolic-ref: %v: %s", err, g.buf.Bytes())
	}
	ref = string(bytes.TrimSpace(b))

	cmd = g.command("git", "show-ref", "--verify", ref)
	if err := cmd.Run(); err == nil {
		g.ref = ref
		return ref, false, nil
	}

	cmd = g.command("git", "branch")
	b, err = cmd.Output()
	if err != nil {
		return "", false, fmt.Errorf("git: failed to run branch: %v: %s", err, g.buf.Bytes())
	}
	if len(b) > 0 {
		return "", false, fmt.Errorf("git: failed to find current branch %s but some branches exists", ref)
	}
	// empty repository no parent commit
	return ref, true, nil
}

func (g *GitRepo) GC() error {
	cmd := exec.Command("git", "gc")
	cmd.Env = g.env
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git: failed to run gc: %v", err)
	}
	return nil
}
