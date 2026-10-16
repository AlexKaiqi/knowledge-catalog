package service

import "kc/kernel"

type fileRange struct {
	offset int64
	length int
}

func requestedFileRange(offset int64, length int) (fileRange, error) {
	if offset < 0 || length < 0 || length > 4<<20 {
		return fileRange{}, kernel.Fail(kernel.ErrUsageInvalid, "file range must use a non-negative offset and length no greater than 4 MiB")
	}
	if length == 0 {
		length = 512 << 10
	}
	return fileRange{offset: offset, length: length}, nil
}

func (r fileRange) slice(content []byte) (int64, []byte, bool) {
	total := int64(len(content))
	start := min(r.offset, total)
	end := start + min(int64(r.length), total-start)
	chunk := content[start:end]
	if chunk == nil {
		chunk = []byte{}
	}
	return start, chunk, end == total
}
