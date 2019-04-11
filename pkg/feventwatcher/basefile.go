package feventwatcher

import (
	"strings"
	"sync"
)

type BaseFile struct {
	NodeName string
	Parent   *BaseFile
	Children map[string]*BaseFile
	mutex    *sync.Mutex
	fullpath string
}

func newBaseFile(nodename string, parent *BaseFile) *BaseFile {
	return &BaseFile{
		NodeName: nodename,
		Parent:   parent,
		Children: make(map[string]*BaseFile),
		mutex:    &sync.Mutex{},
	}
}

func (b *BaseFile) addChildren(pathslices []string) error {
	if len(pathslices) > 0 {
		b.mutex.Lock()
		defer b.mutex.Unlock()

		childname := pathslices[0]
		childbf := b.Children[childname]
		if childbf == nil {
			childbf = newBaseFile(childname, b)
		}
		b.Children[childname] = childbf
		if len(pathslices) > 1 {
			grandchildren := pathslices[1:]
			childbf.addChildren(grandchildren)
		}
	}
	return nil
}

func (b *BaseFile) Fullpath() string {
	if b.fullpath == "" {
		pathslices := []string{b.NodeName}
		for parent := b.Parent; parent != nil; parent = parent.Parent {
			pathslices = append([]string{parent.NodeName}, pathslices...)
		}
		b.fullpath = strings.Join(pathslices, "/")
	}

	return b.fullpath
}
