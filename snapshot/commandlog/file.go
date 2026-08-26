package commandlog

import (
	"os"

	"kc/internal/jsonfile"
)

type FileStore struct {
	file string
}

func NewFileStore(file string) *FileStore { return &FileStore{file: file} }

func (s *FileStore) Load() ([]Entry, error) {
	if _, err := os.Stat(s.file); os.IsNotExist(err) {
		return nil, nil
	}
	var entries []Entry
	if err := jsonfile.Read(s.file, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *FileStore) Save(entries []Entry) error {
	return jsonfile.Write(s.file, entries)
}
