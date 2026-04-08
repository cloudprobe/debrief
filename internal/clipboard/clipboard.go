package clipboard

import (
	"os/exec"
	"strings"
)

// Copy copies s to the system clipboard.
// Returns (tool, true, nil) on success, ("", false, nil) if no clipboard tool found,
// or (tool, false, err) on exec failure.
func Copy(s string) (string, bool, error) {
	return copyWith(s, exec.LookPath, runCmd)
}

func copyWith(s string, lookPath func(string) (string, error), runner func(string, []string, string) error) (string, bool, error) {
	type candidate struct {
		bin  string
		args []string
	}
	candidates := []candidate{
		{"pbcopy", nil},
		{"wl-copy", nil},
		{"xclip", []string{"-selection", "clipboard"}},
	}
	for _, c := range candidates {
		if _, err := lookPath(c.bin); err == nil {
			if err := runner(c.bin, c.args, s); err != nil {
				return c.bin, false, err
			}
			return c.bin, true, nil
		}
	}
	return "", false, nil
}

func runCmd(bin string, args []string, stdin string) error {
	cmd := exec.Command(bin, args...)
	cmd.Stdin = strings.NewReader(stdin)
	return cmd.Run()
}
