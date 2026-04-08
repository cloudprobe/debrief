package clipboard

import (
	"errors"
	"testing"
)

const pbcopyBin = "pbcopy"

func notFound(string) (string, error) {
	return "", errors.New("not found")
}

func successRunner(bin string, args []string, stdin string) error {
	return nil
}

func failRunner(bin string, args []string, stdin string) error {
	return errors.New("exec failed")
}

func TestCopy_NoToolFound(t *testing.T) {
	tool, ok, err := copyWith("hello", notFound, successRunner)
	if tool != "" || ok || err != nil {
		t.Errorf("expected (\"\", false, nil), got (%q, %v, %v)", tool, ok, err)
	}
}

func TestCopy_PbcopySuccess(t *testing.T) {
	lookPath := func(bin string) (string, error) {
		if bin == pbcopyBin {
			return "/usr/bin/pbcopy", nil
		}
		return "", errors.New("not found")
	}
	tool, ok, err := copyWith("hello", lookPath, successRunner)
	if tool != pbcopyBin || !ok || err != nil {
		t.Errorf("expected (\"pbcopy\", true, nil), got (%q, %v, %v)", tool, ok, err)
	}
}

func TestCopy_WlCopyFallback(t *testing.T) {
	lookPath := func(bin string) (string, error) {
		if bin == "wl-copy" {
			return "/usr/bin/wl-copy", nil
		}
		return "", errors.New("not found")
	}
	tool, ok, err := copyWith("hello", lookPath, successRunner)
	if tool != "wl-copy" || !ok || err != nil {
		t.Errorf("expected (\"wl-copy\", true, nil), got (%q, %v, %v)", tool, ok, err)
	}
}

func TestCopy_XclipFallback(t *testing.T) {
	lookPath := func(bin string) (string, error) {
		if bin == "xclip" {
			return "/usr/bin/xclip", nil
		}
		return "", errors.New("not found")
	}
	tool, ok, err := copyWith("hello", lookPath, successRunner)
	if tool != "xclip" || !ok || err != nil {
		t.Errorf("expected (\"xclip\", true, nil), got (%q, %v, %v)", tool, ok, err)
	}
}

func TestCopy_RunnerError(t *testing.T) {
	lookPath := func(bin string) (string, error) {
		if bin == pbcopyBin {
			return "/usr/bin/pbcopy", nil
		}
		return "", errors.New("not found")
	}
	tool, ok, err := copyWith("hello", lookPath, failRunner)
	if tool != pbcopyBin || ok || err == nil {
		t.Errorf("expected (\"pbcopy\", false, err), got (%q, %v, %v)", tool, ok, err)
	}
}
