package palworld

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// levelSavPath names the world file: Level.sav in the save directory
// (or the directory itself already being the file).
func levelSavPath(savePath string) (string, error) {
	if strings.EqualFold(filepath.Base(savePath), "Level.sav") {
		return savePath, nil
	}
	p := filepath.Join(savePath, "Level.sav")
	if _, err := os.Stat(p); err != nil {
		return "", errors.New("no Level.sav in the save directory — has the server saved yet?")
	}
	return p, nil
}

// verifySavMagic rejects a Level.sav that doesn't look like a Palworld
// save container — most importantly one caught mid-write, which would
// otherwise archive fine and only reveal itself on restore day.
func verifySavMagic(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	head := make([]byte, 24)
	if _, err := io.ReadFull(f, head); err != nil {
		return fmt.Errorf("save too short to be valid: %w", err)
	}
	magic := string(head[8:11])
	if magic == "PlZ" || magic == "PlM" {
		return nil
	}
	// Xbox-style chunked container: the real header sits 12 bytes in.
	if string(head[8:11]) == "CNK" {
		inner := string(head[20:23])
		if inner == "PlZ" || inner == "PlM" {
			return nil
		}
	}
	return errors.New("Level.sav doesn't look like a Palworld save (mid-write? wrong path?)")
}
