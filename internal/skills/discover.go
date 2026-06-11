package skills

import (
	"io/fs"
	"path/filepath"
	"sort"
	"sync"

	"github.com/charlievieth/fastwalk"
)

type Discoverer struct{}

func NewDiscoverer() *Discoverer {
	return &Discoverer{}
}

func Discover(paths []string) []*Skill {
	return NewDiscoverer().Discover(paths)
}

func (d *Discoverer) Discover(paths []string) []*Skill {
	var mu sync.Mutex
	skills := make([]*Skill, 0)
	seenFiles := make(map[string]struct{})
	conf := fastwalk.Config{Follow: true}

	for _, root := range paths {
		expanded := ExpandPath(root, "")
		_ = fastwalk.Walk(&conf, expanded, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry == nil || entry.Name() != SkillFileName {
				return nil
			}

			realFile, err := filepath.EvalSymlinks(path)
			if err != nil {
				return nil
			}

			mu.Lock()
			if _, ok := seenFiles[realFile]; ok {
				mu.Unlock()
				return nil
			}
			seenFiles[realFile] = struct{}{}
			mu.Unlock()

			skill, err := Parse(realFile)
			if err != nil || skill.Validate() != nil {
				return nil
			}

			mu.Lock()
			skills = append(skills, skill)
			mu.Unlock()
			return nil
		})
	}

	sort.SliceStable(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})
	return skills
}
