package control

import (
	"encoding/json"
	bolt "go.etcd.io/bbolt"
	index "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
	"time"
)

type stateNoticeRecord struct {
	index.ChangeNotice
	Generation uint64 `json:"generation"`
	Status     string `json:"status"`
	LastError  string `json:"lastError,omitempty"`
	UpdatedAt  string `json:"updatedAt"`
}

func noticeKey(notice index.ChangeNotice) []byte {
	addr := "*"
	if notice.Address != nil {
		addr = objectmodel.AddressKey(*notice.Address)
	}
	return []byte(string(notice.Repository) + "\x00" + snapshot.RefOrDefault(notice.Ref) + "\x00" + addr)
}

func (s *TargetStore) enqueueNotice(notice index.ChangeNotice) error {
	return s.update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(stateNoticeBucket)
		if err != nil {
			return err
		}
		record := stateNoticeRecord{ChangeNotice: notice, Status: index.TargetPending}
		if raw := bucket.Get(noticeKey(notice)); raw != nil {
			if err := json.Unmarshal(raw, &record); err != nil {
				return err
			}
			record.ChangeNotice = notice
		}
		generation, err := bucket.NextSequence()
		if err != nil {
			return err
		}
		if generation == 0 {
			return kernel.Fail(kernel.ErrPreconditionFailed, "State notice generation exhausted")
		}
		record.Generation = generation
		record.Status = index.TargetPending
		record.LastError = ""
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		raw, err := json.Marshal(record)
		if err != nil {
			return err
		}
		return bucket.Put(noticeKey(notice), raw)
	})
}

func (s *TargetStore) pendingNotices() ([]stateNoticeRecord, error) {
	notices := []stateNoticeRecord{}
	err := s.view(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateNoticeBucket)
		if bucket == nil {
			return nil
		}
		return bucket.ForEach(func(_, raw []byte) error {
			var record stateNoticeRecord
			if err := json.Unmarshal(raw, &record); err != nil {
				return err
			}
			if record.Status == index.TargetReady {
				return nil
			}
			notices = append(notices, record)
			return nil
		})
	})
	return notices, err
}

func (s *TargetStore) finishNotice(attempted stateNoticeRecord, applyErr error) error {
	return s.update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateNoticeBucket)
		if bucket == nil {
			return nil
		}
		key := noticeKey(attempted.ChangeNotice)
		raw := bucket.Get(key)
		if raw == nil {
			return nil
		}
		var record stateNoticeRecord
		if err := json.Unmarshal(raw, &record); err != nil {
			return err
		}
		// A same-key notice may arrive during external observation. Only the
		// exact durable generation attempted here may finish or fail that work.
		if record.Generation != attempted.Generation {
			return nil
		}
		if applyErr == nil {
			return bucket.Delete(key)
		}
		record.Status = index.TargetFailed
		record.LastError = applyErr.Error()
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		next, err := json.Marshal(record)
		if err != nil {
			return err
		}
		return bucket.Put(key, next)
	})
}
