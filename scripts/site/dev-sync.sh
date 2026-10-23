# Remote payload for `deploy.sh dev sync`, executed on the dev machine via
# `bash -s -- <dir> <home>` with the tar archive on stdin.
#
# Order matters: the archive is extracted FIRST, because a payload that reads
# stdin for anything else would swallow the archive. Everything after that
# (AppleDouble cleanup, Ollama-free hashing) runs in the same round trip.
set -u

dir=${1:?usage: dev-sync.sh <dir> <home>}
home=${2:?}
me=$(cd -- "$(dirname -- "$0")" 2>/dev/null && pwd || true)

mkdir -p "$dir" "$home" || { echo "cannot create $dir / $home" >&2; exit 1; }
if ! tar xf - -C "$dir"; then
    echo "archive extraction failed" >&2
    exit 1
fi

# AppleDouble files (._name) are macOS metadata, not site content. Leaving them
# behind makes the two trees impossible to compare by hash.
find "$dir" -name '._*' -type f -delete 2>/dev/null || true
find "$dir" -name '.DS_Store' -type f -delete 2>/dev/null || true

python3 - "$dir" <<'PY'
import hashlib, os, sys
root = sys.argv[1]
rows = []
for base, dirs, files in os.walk(root):
    dirs[:] = sorted(d for d in dirs if d != '.git')
    for name in sorted(files):
        if name == '.DS_Store' or name.startswith('._'):
            continue
        path = os.path.join(base, name)
        if not os.path.isfile(path) or os.path.islink(path):
            continue
        rel = os.path.relpath(path, root)
        digest = hashlib.sha256()
        with open(path, 'rb') as handle:
            for chunk in iter(lambda: handle.read(1 << 20), b''):
                digest.update(chunk)
        rows.append('%s\t%d\t%s' % (rel, os.path.getsize(path), digest.hexdigest()))
print(hashlib.sha256('\n'.join(rows).encode()).hexdigest())
PY
