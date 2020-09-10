#!/bin/bash
set -euxo pipefail

echo "hello from bash script"

env

sudo apt-get install jq

remove_label() {
	local url=$(echo $GITHUB_CONTEXT | jq --raw-output .event.pull_request._links.issue.href)/labels/merge-me

	curl -X DELETE -H "Accept: application/vnd.github.v3+json" \
		--header "authorization: Bearer $GITHUB_TOKEN" "$url"
}

add_comment() {
	local comment="$1"
	local url=$(echo $GITHUB_CONTEXT | jq --raw-output .event.pull_request._links.comments.href)
	local data=$(jq --null-input --arg body "$comment" '{ body: $body }')

	curl --request POST --url "$url" \
		--header "authorization: Bearer $GITHUB_TOKEN" \
		--header 'content-type: application/json' \
		--data "$data"
}

add_comment "testing leaving a comment"
remove_label
