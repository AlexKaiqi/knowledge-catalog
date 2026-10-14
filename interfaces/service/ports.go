package service

import (
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

type fixedDeclarations struct{ scope *reader.Serving }

func (r fixedDeclarations) ReadObject(repository kernel.RepositoryID, id objectmodel.ObjectID) (objectmodel.KnowledgeValue, error) {
	repo, err := r.scope.Member(repository)
	if err != nil {
		return objectmodel.KnowledgeValue{}, err
	}
	at := r.scope.Pin().Repositories[repository]
	value, err := repo.Read(id, at)
	if err == nil {
		err = objectmodel.ValidateHydratedObject(repository, at, id, value)
	}
	return value, err
}

func (r fixedDeclarations) ReadAddress(repository kernel.RepositoryID, address objectmodel.Address) (objectmodel.KnowledgeValue, error) {
	repo, err := r.scope.Member(repository)
	if err != nil {
		return objectmodel.KnowledgeValue{}, err
	}
	at := r.scope.Pin().Repositories[repository]
	value, err := repo.ReadAddress(address, at)
	if err == nil {
		err = objectmodel.ValidateHydratedAddress(repository, at, address, value)
	}
	return value, err
}
