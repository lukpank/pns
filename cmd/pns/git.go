// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const gitChunkSize = 1000 // assumed to be at least 100 and multiple of 100

type GitRepo struct {
	dir string
	env []string
	ref string
	buf bytes.Buffer

	// tree of trees and blobs and last commit
	blobsCnt  int
	treesCnt  int
	blobs     [][]SHA1
	trees     [][]SHA1
	emptySHA1 SHA1 // left at its zero value
	author    string

	indexBuf bytes.Buffer
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

type SHA1 [20]byte

type objectType int

const (
	objectBlob = iota
	objectTree
	objectCommit
)

func (g *GitRepo) hashObject(typ objectType, b []byte) (h SHA1, err error) {
	var buf bytes.Buffer
	switch typ {
	case objectBlob:
		fmt.Fprintf(&buf, "blob %d\x00", len(b))
	case objectTree:
		fmt.Fprintf(&buf, "tree %d\x00", len(b))
	case objectCommit:
		fmt.Fprintf(&buf, "commit %d\x00", len(b))
	default:
		err = fmt.Errorf("unsupported object type %d", typ)
		return
	}
	h1 := sha1.New()
	h1.Write(buf.Bytes())
	h1.Write(b)
	sum := h1.Sum(h[:0])
	hex.EncodeToString(sum[:1])
	dirName := filepath.Join(g.dir, "objects", hex.EncodeToString(sum[:1]))
	fileName := filepath.Join(dirName, hex.EncodeToString(sum[1:]))
	tmpFileName := fileName + ".tmp"
	if _, err := os.Stat(fileName); err == nil || err != nil && !os.IsNotExist(err) {
		return h, err
	}
	if err := os.MkdirAll(dirName, 0777); err != nil {
		return h, err
	}
	f, err := os.Create(tmpFileName)
	defer f.Close()
	if err != nil {
		return h, err
	}
	w := zlib.NewWriter(f) // TODO: w.Reset(newWriter)
	_, err = w.Write(buf.Bytes())
	if err == nil {
		_, err = w.Write(b)
	}
	if err == nil {
		err = w.Close()
	}
	if err == nil {
		err = f.Close()
	}
	if err != nil {
		os.Remove(tmpFileName) // forget error we already have one
		return
	}
	err = os.Rename(tmpFileName, fileName)
	return h, err
}

func (g *GitRepo) addWriteTree(id int, h SHA1) (SHA1, error) {
	for id >= len(g.blobs)*gitChunkSize {
		g.blobs = append(g.blobs, make([]SHA1, gitChunkSize))
	}
	for id/100 >= len(g.trees)*gitChunkSize {
		g.trees = append(g.trees, make([]SHA1, gitChunkSize))
	}
	if m := id + 1; m > g.blobsCnt {
		g.blobsCnt = m
	}
	if m := id/100 + 1; m > g.treesCnt {
		g.treesCnt = m
	}
	g.blobs[id/gitChunkSize][id%gitChunkSize] = h
	if id < 100 {
		return g.writeTrees0()
	}
	for i := id / 100; i > 0; i /= 100 {
		h, err := g.writeTreesN(i)
		if err != nil {
			return h, err
		}
		g.trees[i/gitChunkSize][i%gitChunkSize] = h
	}
	n := intMin(g.treesCnt, 100)
	return g.writeTree(nil, g.trees[0][:n])
}

func (g *GitRepo) writeTrees0() (SHA1, error) {
	n := intMin(g.blobsCnt, 100)
	h, err := g.writeTree(g.blobs[0][:n], nil)
	if err != nil {
		return SHA1{}, err
	}
	g.trees[0][0] = h
	n = intMin(g.treesCnt, 100)
	return g.writeTree(nil, g.trees[0][:n])
}

func (g *GitRepo) writeTreesN(num int) (SHA1, error) {
	var b, t []SHA1
	if num*100 < g.blobsCnt {
		k := num * 100 / gitChunkSize
		n := intMin((num+1)*100, g.blobsCnt) % gitChunkSize
		if n == 0 {
			n = gitChunkSize
		}
		b = g.blobs[k][(num*100)%gitChunkSize : n]
	}
	if num*100 < g.treesCnt {
		k := num * 100 / gitChunkSize
		n := intMin((num+1)*100, g.treesCnt)
		if n == 0 {
			n = gitChunkSize
		}
		t = g.trees[k][(num*100)%gitChunkSize : n]
	}
	return g.writeTree(b, t)
}

func (g *GitRepo) writeTree(blobs, trees []SHA1) (SHA1, error) {
	n := intMax(len(blobs), len(trees))
	g.buf.Reset()
	for i := 0; i < n; i++ {
		if i < len(blobs) && blobs[i] != g.emptySHA1 {
			fmt.Fprintf(&g.buf, "100644 %02d.md\x00", i)
			g.buf.Write(blobs[i][:])
		}
		if i < len(trees) && trees[i] != g.emptySHA1 {
			fmt.Fprintf(&g.buf, "40000 %02d\x00", i)
			g.buf.Write(trees[i][:])
		}
	}
	return g.hashObject(objectTree, g.buf.Bytes())
}

func (g *GitRepo) writeCommit(id int, tree, parent SHA1, authorDate time.Time) (SHA1, error) {
	if g.author == "" {
		cmd := g.command("git", "config", "--get", "user.name")
		b1, err := cmd.Output()
		if err != nil {
			return SHA1{}, fmt.Errorf("git: failed to get config user.name: %v: %s", err, g.buf.Bytes())
		}
		cmd = g.command("git", "config", "--get", "user.email")
		b2, err := cmd.Output()
		if err != nil {
			return SHA1{}, fmt.Errorf("git: failed to get config user.email: %v: %s", err, g.buf.Bytes())
		}
		g.buf.Write(bytes.TrimSpace(b1))
		g.buf.WriteString(" <")
		g.buf.Write(bytes.TrimSpace(b2))
		g.buf.WriteByte('>')
		g.author = g.buf.String()
	}

	g.buf.Reset()
	fmt.Fprintf(&g.buf, "tree %x\n", tree)
	if parent != g.emptySHA1 {
		fmt.Fprintf(&g.buf, "parent %x\n", parent)
	}
	t := time.Now()
	fmt.Fprintf(&g.buf, "author %s %d %s\n", g.author, authorDate.Unix(), authorDate.Format("-0700"))
	fmt.Fprintf(&g.buf, "committer %s %d %s\n\n%d\n", g.author, t.Unix(), t.Format("-0700"), id)
	return g.hashObject(objectCommit, g.buf.Bytes())
}

func (g *GitRepo) addToIndex(path string, hash SHA1) {
	fmt.Fprintf(&g.indexBuf, "100644 %x\t%s\n", hash, path)
}

func (g *GitRepo) updateIndex() error {
	cmd := g.command("git", "update-index", "--index-info")
	cmd.Stdin = &g.indexBuf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git: failed to run update-index: %v: %s", err, g.buf.Bytes())
	}
	g.indexBuf.Reset()
	return nil
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

func intMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func intMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}
