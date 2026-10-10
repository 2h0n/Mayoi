#!/usr/bin/env bash
# Sets a new version in mygo.json, commits it as v<version> with a tag of the
# same name, and after asking pushes main and the tag, which makes the
# Release workflow build and publish it.
# Usage: scripts/bump.sh <version | patch | minor | major>
#
# CHANGELOG.md needs a "## <version>" section first: the update window shows
# it, and the release fails without it. Pre-release versions such as
# 1.2.0-beta.1 are refused, as Mayoi has no beta channel yet.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
if [ $# -ne 1 ]; then
    sed -n '2,9s/^# \{0,1\}//p' "$0" >&2
    exit 1
fi

cd "$ROOT"
CURRENT="$(/usr/bin/python3 -c 'import json; print(json.load(open("mygo.json"))["version"])')"
# Prints the new version, or fails when the argument isn't a version or
# keyword or doesn't come after the current version.
VERSION="$(/usr/bin/python3 - "$CURRENT" "$1" <<'PY'
import re, sys

def parse(v):
    m = re.fullmatch(r"(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)", v)
    if not m:
        sys.exit(f"{v} isn't a version such as 1.2.0.")
    return tuple(int(x) for x in m.groups())

current, arg = parse(sys.argv[1]), sys.argv[2]
major, minor, patch = current
new = {
    "patch": (major, minor, patch + 1),
    "minor": (major, minor + 1, 0),
    "major": (major + 1, 0, 0),
}.get(arg) or parse(arg)
if new <= current:
    sys.exit("%d.%d.%d doesn't come after the current version %s." % (*new, sys.argv[1]))
print("%d.%d.%d" % new)
PY
)"
TAG="v$VERSION"

if [ "$(git symbolic-ref --short HEAD 2>/dev/null)" != main ]; then
    echo "Releases are made from main. Switch to it first." >&2
    exit 1
fi
if [ -n "$(git status --porcelain)" ]; then
    echo "The working tree has changes. Commit them first." >&2
    exit 1
fi
git fetch --quiet --tags origin main
if [ "$(git rev-parse HEAD)" != "$(git rev-parse origin/main)" ]; then
    echo "main differs from origin/main. Pull or push first." >&2
    exit 1
fi
if git rev-parse --verify --quiet "refs/tags/$TAG" >/dev/null \
    || [ -n "$(git ls-remote --tags origin "refs/tags/$TAG")" ]; then
    echo "Tag $TAG exists already." >&2
    exit 1
fi
if gh release view "$TAG" >/dev/null 2>&1; then
    echo "Release $TAG exists already." >&2
    exit 1
fi
if ! grep -q "^## $VERSION\$" CHANGELOG.md; then
    echo "CHANGELOG.md has no \"## $VERSION\" section. Add one and commit it first." >&2
    exit 1
fi

echo "==> $CURRENT -> $VERSION"
/usr/bin/python3 - "$VERSION" <<'PY'
import json, sys
with open("mygo.json") as f:
    config = json.load(f)
config["version"] = sys.argv[1]
with open("mygo.json", "w") as f:
    json.dump(config, f, indent=2, ensure_ascii=False)
    f.write("\n")
PY
if [ "$(git diff --name-only)" != mygo.json ] \
    || [ "$(git diff -U0 mygo.json | grep -c '^[-+] ')" != 2 ]; then
    echo "Expected only the version in mygo.json to change:" >&2
    git diff >&2
    echo "Undo with: git checkout -- mygo.json" >&2
    exit 1
fi
git commit --quiet -m "$TAG" mygo.json
git tag "$TAG"
git --no-pager log -1 --stat --format='%h %s' "$TAG"

read -r -p "Push main and $TAG to origin, which publishes the release? [y/N] " answer || echo
if [[ "$answer" != [yY]* ]]; then
    echo "Not pushed. Push later with: git push --atomic origin main $TAG"
    echo "Or undo with: git tag -d $TAG && git reset --hard HEAD~1"
    exit 0
fi
git push --quiet --atomic origin main "$TAG"
echo "==> pushed $TAG. The Release workflow: https://github.com/sorrycc/Mayoi/actions/workflows/release.yml"
