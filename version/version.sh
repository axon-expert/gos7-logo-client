#!/bin/bash

if [ -z "$NO_PUSH" ]; then
    if [ -n "$(git status --porcelain)" ]; then
        echo "Pleas commit or stash all your changes first"
        exit 2
    fi
fi

VERSION_FILE="./version"
VERSION_PREFIX="v"
CURRENT_VERSION=$(cat "$VERSION_FILE")

if [[ "$CURRENT_VERSION" =~ ^$VERSION_PREFIX([0-9]+)\.([0-9]+)\.([0-9]+)-?([a-z]([0-9]+))?$ ]]; then
    MAJOR=${BASH_REMATCH[1]}
    MINOR=${BASH_REMATCH[2]}
    PATCH=${BASH_REMATCH[3]}
    PRE_RELEASE=${BASH_REMATCH[4]}
    PRE_NUM=${BASH_REMATCH[5]}
else
    echo "Invalid version format: $CURRENT_VERSION"
    exit 1
fi

case $1 in
    manual)
        if [ -z "$2" ]; then
            echo "Manual version required. Usage: $0 manual <version>"
            exit 1
        fi
        NEW_VERSION=$2
        ;;

    major)
        if [ -z "$PRE_RELEASE" ]; then
            NEW_VERSION="$VERSION_PREFIX$((MAJOR + 1)).0.0"
        elif [[ "$MINOR" == 0 && "$PATCH" == 0 ]]; then
            NEW_VERSION="$VERSION_PREFIX$MAJOR.0.0"
        else
            NEW_VERSION="$VERSION_PREFIX$((MAJOR + 1)).0.0"
        fi
        ;;

    minor)
        if [ -z "$PRE_RELEASE" ]; then
            NEW_VERSION="$VERSION_PREFIX$MAJOR.$((MINOR + 1)).0"
        elif [[ "$MINOR" == 0 ]]; then
            NEW_VERSION="$VERSION_PREFIX$MAJOR.1.0"
        elif [[ "$PATCH" == 0 ]]; then
            NEW_VERSION="$VERSION_PREFIX$MAJOR.$MINOR.0"
        else
            NEW_VERSION="$VERSION_PREFIX$MAJOR.$((MINOR + 1)).0"
        fi
        ;;

    patch)
        if [ -z "$PRE_RELEASE" ]; then
            NEW_VERSION="$VERSION_PREFIX$MAJOR.$MINOR.$((PATCH + 1))"
        elif [[ "$PATCH" == 0 ]]; then
            NEW_VERSION="$VERSION_PREFIX$MAJOR.$MINOR.1"
        else
            NEW_VERSION="$VERSION_PREFIX$MAJOR.$MINOR.$PATCH"
        fi
        ;;

    premajor)
        if [ -z "$PRE_RELEASE" ]; then
            NEW_VERSION="$VERSION_PREFIX$((MAJOR + 1)).0.0-a0"
        elif [[ "$MINOR" != 0 || "$PATCH" != 0 || "$MAJOR" == 0 ]]; then
            NEW_VERSION="$VERSION_PREFIX$((MAJOR + 1)).0.0-${PRE_RELEASE:0:1}$((PRE_NUM + 1))"
        else
            NEW_VERSION="$VERSION_PREFIX$MAJOR.0.0-${PRE_RELEASE:0:1}$((PRE_NUM + 1))"
        fi
        ;;

    preminor)
        if [ -z "$PRE_RELEASE" ]; then
            NEW_VERSION="$VERSION_PREFIX$MAJOR.$((MINOR + 1)).0-a0"
        elif [[ "$PATCH" != 0 || "$MINOR" == 0 ]]; then
            NEW_VERSION="$VERSION_PREFIX$MAJOR.$((MINOR + 1)).0-${PRE_RELEASE:0:1}$((PRE_NUM + 1))"
        else
            NEW_VERSION="$VERSION_PREFIX$MAJOR.$MINOR.0-${PRE_RELEASE:0:1}$((PRE_NUM + 1))"
        fi
        ;;

    prepatch)
        if [ -z "$PRE_RELEASE" ]; then
            NEW_VERSION="$VERSION_PREFIX$MAJOR.$MINOR.$((PATCH + 1))-a0"
        elif [[ "$PATCH" == 0 ]]; then
            NEW_VERSION="$VERSION_PREFIX$MAJOR.$MINOR.1-${PRE_RELEASE:0:1}$((PRE_NUM + 1))"
        else
            NEW_VERSION="$VERSION_PREFIX$MAJOR.$MINOR.$PATCH-${PRE_RELEASE:0:1}$((PRE_NUM + 1))"
        fi
        ;;

    prerelease)
        if [ -z "$PRE_RELEASE" ]; then
            NEW_VERSION="$VERSION_PREFIX$MAJOR.$MINOR.$((PATCH + 1))-a0"
        else
            NEW_VERSION="$VERSION_PREFIX$MAJOR.$MINOR.$PATCH-${PRE_RELEASE:0:1}$((PRE_NUM + 1))"
        fi
        ;;

    *)
        echo "Invalid version type: $1"
        echo "Usage: $0 {major|minor|patch|prerelease|manual <version>}"
        exit 1
        ;;
esac

echo "$NEW_VERSION" > "$VERSION_FILE"

if [ -n "$NO_PUSH" ]; then
    echo "$NEW_VERSION"
    exit 0
fi

if [ -z "$(git config --get branch."$(git branch --show-current)".remote)" ]; then
    git push --set-upstream origin "$(git branch --show-current)"
fi

git add "$VERSION_FILE"
git commit -m "$NEW_VERSION"
git push

if git rev-parse "$NEW_VERSION" >/dev/null 2>&1; then
    echo "Tag $NEW_VERSION already exists. Skipping release creation."
else
    PRERELEASE_FLAG=""
    if [[ "$NEW_VERSION" =~ [a-z][0-9]+$ ]]; then
        PRERELEASE_FLAG="--prerelease"
    fi

    gh release create "$NEW_VERSION" \
        --generate-notes \
        --target="$(git branch --show-current)" \
        $PRERELEASE_FLAG

    git fetch --tags
fi

echo "$CURRENT_VERSION -> $NEW_VERSION"
