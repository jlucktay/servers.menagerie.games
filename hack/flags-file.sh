#!/usr/bin/env bash
set -euo pipefail

project_root=$(git rev-parse --show-toplevel)

if ! [ -r "$project_root"/.env ]; then
  echo >&2 "can not read '.env' file in project root directory '$project_root'"
  exit 1
fi

output="--set-env-vars: "

while read -r kv; do
  key=${kv%=*}

  if ! [[ $key =~ "^SMG_" ]]; then
    key="SMG_$key"
  fi

  output+="$key=${kv##*=},"
done < "$project_root"/.env

mkdir -pv "$project_root"/tmp
echo "${output%,*}" > "$project_root"/tmp/flags.yaml
