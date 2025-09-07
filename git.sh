#!/bin/bash

# If any command fails, stop the whole script
set -e

# Name the commit message as str
str="$*"
if [ -z "$str" ]; then
  echo "❌ Error: Commit message is required."
  exit 1
fi

# Format Go code using gofumpt
gofumpt -l -w .

# Stage all changes
git add .

# Skip if nothing changed
if git diff --cached --quiet && git diff --quiet; then
  echo "❗ Nothing to commit."
  exit 0
fi

# Commit with provided message
git commit -m "$str"

# Push to remote
git push
