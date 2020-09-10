#!/bin/bash
set -euxo pipefail

echo "hello from bash script"

env

sudo apt-get install jq

remove_label() {
	url=$(echo $GITHUB_CONTEXT | jq --raw-output .event.pull_request._links.issue.href)/labels/merge-me
	curl -X DELETE -H "Accept: application/vnd.github.v3+json" \
		--header "authorization: Bearer $GITHUB_TOKEN" "$url"
}

remove_label
