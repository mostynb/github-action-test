#!/bin/bash
set -euo pipefail

env

sudo apt-get install jq curl

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

close_issue() {
	local url=$(echo $GITHUB_CONTEXT | jq --raw-output .event.pull_request._links.issue.href)

	curl -X PATCH -H "Accept: application/vnd.github.v3+json" \
		--header "authorization: Bearer $GITHUB_TOKEN" $url \
		-d '{"state": "closed"}'
}

check_prereqs() {
	local merged="$(echo $GITHUB_CONTEXT | jq --raw-output .event.pull_request.merged)"
	if [ "$merged" != false ]
	then
		add_comment "PR is already merged"
		remove_label
		exit 1
	fi

	local mergeable="$(echo $GITHUB_CONTEXT | jq --raw-output .event.pull_request.mergeable)"
	if [ "$mergeable" != true ]
	then
		add_comment "PR does not seem to be mergeable"
		remove_label
		exit 1
	fi

	local base_ref="$(echo $GITHUB_CONTEXT | jq --raw-output .base_ref)"
	if [ "$base_ref" != master ]
	then
		add_comment "PR is not based on master"
		remove_label
		exit 1
	fi
}

run_hooks() {
	# Run some pretend "hooks" that might commit something.

	date >> commit_log.txt
	echo "hooks run :)" >> commit_log.txt

	git config user.name "GitHub Actions Bot"
	git config user.email "<>"
	git add commit_log.txt

	if git commit -m "run hooks"
	then
		git push origin master
	fi

	# TODO: Add a link to the pushed commit?
	add_comment "Pushed to master"
	remove_label
	close_issue
}

check_prereqs
run_hooks

