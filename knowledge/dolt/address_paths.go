package dolt

import (
	"kc/kernel"
	"kc/knowledge"
	"slices"
)

func (r *Repository) checkAddressPaths(address knowledge.Address, commit kernel.CommitID, paths []string) error {
	if err := knowledge.AssertWritable(address); err != nil {
		return err
	}
	if !r.HasCommit(commit) {
		return kernel.Fail(kernel.ErrVersionUnresolved, "address commit is missing")
	}
	rows, err := r.base.NativeQuery("SELECT TO_BASE64(CAST(path_hint AS BINARY)) AS path64 FROM kc_units AS OF " + sqlString(string(commit)) + " WHERE unit_key=" + sqlString(unitKey(address)) + " LIMIT 1")
	if err != nil {
		return err
	}
	if len(rows) != 1 {
		return kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "address is missing")
	}
	path, err := rowText64(rows[0], "path64")
	if err != nil {
		return err
	}
	if path == "" || !slices.Contains(paths, path) {
		return kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "address is outside selected file paths")
	}
	return nil
}

func (r *Repository) ReadAddressAtPaths(address knowledge.Address, commit kernel.CommitID, paths []string) (knowledge.KnowledgeValue, error) {
	if err := r.checkAddressPaths(address, commit, paths); err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	return r.ReadAddress(address, commit)
}

func (r *Repository) ResolveAddressAtPaths(address knowledge.Address, commit kernel.CommitID, paths []string) (knowledge.Resolution, error) {
	if err := r.checkAddressPaths(address, commit, paths); err != nil {
		return knowledge.Resolution{}, err
	}
	return r.ResolveAddress(address, commit)
}
