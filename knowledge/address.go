package knowledge

import (
	"strings"

	"kc/kernel"
)

type AddressKind string

const (
	KindEntity   AddressKind = "Entity"
	KindAspect   AddressKind = "Aspect"
	KindMember   AddressKind = "Member"
	KindRelation AddressKind = "Relation"
	KindRecord   AddressKind = "Record"
)

// Address identifies one independently maintained unit of knowledge. It is
// not a Snapshot path and never participates in Catalog workspace pins.
type Address struct {
	Kind       AddressKind `json:"kind"`
	ObjectID   ObjectID    `json:"objectId"`
	AspectName string      `json:"aspectName,omitempty"`
	MemberKey  string      `json:"memberKey,omitempty"`
}

const addressSep = "\u001f"

func AddressKey(a Address) string {
	return string(a.ObjectID) + addressSep + a.AspectName + addressSep + a.MemberKey
}

func IsEntityBlob(a Address) bool {
	return a.AspectName == "" && a.MemberKey == ""
}

func InferAddress(objectID ObjectID, aspectName, memberKey, kindField string) Address {
	if memberKey != "" && aspectName != "" {
		return Address{Kind: KindMember, ObjectID: objectID, AspectName: aspectName, MemberKey: memberKey}
	}
	if aspectName != "" {
		kind := KindAspect
		if kindField == string(KindRecord) {
			kind = KindRecord
		}
		return Address{Kind: kind, ObjectID: objectID, AspectName: aspectName}
	}
	kind := KindEntity
	if kindField == string(KindRelation) {
		kind = KindRelation
	}
	return Address{Kind: kind, ObjectID: objectID}
}

func AssertWritable(a Address) error {
	aspect := strings.TrimSpace(a.AspectName)
	member := strings.TrimSpace(a.MemberKey)
	switch a.Kind {
	case KindEntity, KindRelation:
		if aspect != "" || member != "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "%s address cannot carry aspectName/memberKey", a.Kind)
		}
	case KindAspect, KindRecord:
		if aspect == "" || member != "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "%s address requires aspectName and no memberKey", a.Kind)
		}
	case KindMember:
		if aspect == "" || member == "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "Member address requires aspectName and memberKey")
		}
	default:
		return kernel.Fail(kernel.ErrUsageInvalid, "unknown address kind %s", a.Kind)
	}
	return nil
}
