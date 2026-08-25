package gitdir

import "strings"

// Paths lists every blob path in a commit tree.
func (d *Dir) Paths(rev string) ([]string, error) {
	raw, err := d.Git("ls-tree", "-r", "--name-only", rev)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out, nil
}

func (d *Dir) Show(rev, path string) (string, error) {
	return d.Git("show", rev+":"+path)
}

// ShowRaw reads one blob without trimming trailing whitespace.
func (d *Dir) ShowRaw(rev, path string) ([]byte, error) {
	return d.gitRaw("show", rev+":"+path)
}

// ObjectType reports the git object type at path in rev.
func (d *Dir) ObjectType(rev, path string) (kind string, ok bool) {
	out, err := d.Git("cat-file", "-t", rev+":"+path)
	if err != nil {
		return "", false
	}
	return out, true
}
