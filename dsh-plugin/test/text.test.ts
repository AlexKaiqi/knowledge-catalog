import { describe, expect, it } from 'vitest';
import { applyLiteralEdit, AmbiguousEditError, decodeStrictText, EditNotFoundError, NotTextError } from '../src/text.js';

describe('decodeStrictText', () => {
  it('decodes valid UTF-8', () => {
    expect(decodeStrictText(new TextEncoder().encode('hello\n'), 'a.md')).toBe('hello\n');
  });

  it('rejects a null byte in the sample as binary, not as replacement characters', () => {
    const bytes = new Uint8Array([0x68, 0x69, 0x00, 0x6a]);
    expect(() => decodeStrictText(bytes, 'a.bin')).toThrow(NotTextError);
  });

  it('rejects invalid UTF-8 instead of silently replacing it', () => {
    const bytes = new Uint8Array([0xff, 0xfe, 0xfd]);
    expect(() => decodeStrictText(bytes, 'a.bin')).toThrow(NotTextError);
  });
});

describe('applyLiteralEdit', () => {
  it('replaces exactly one match', () => {
    expect(applyLiteralEdit('a b a', 'b', 'c', false, 'f.md')).toBe('a c a');
  });

  it('requires replaceAll for more than one match', () => {
    expect(() => applyLiteralEdit('a a', 'a', 'x', false, 'f.md')).toThrow(AmbiguousEditError);
    expect(applyLiteralEdit('a a', 'a', 'x', true, 'f.md')).toBe('x x');
  });

  it('a string that does not occur is not found, not a silent no-op', () => {
    expect(() => applyLiteralEdit('abc', 'zzz', 'x', false, 'f.md')).toThrow(EditNotFoundError);
  });

  it('an empty newString deletes the matched text', () => {
    expect(applyLiteralEdit('a-b', '-', '', false, 'f.md')).toBe('ab');
  });
});
