#!/bin/bash
set -euo pipefail
set -x

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

bail_out() {
	local msg="something went wrong trying to merge"
	if [ -n "$1" ]
	then
		add_comment "$msg: $1"
	else
		add_comment "$msg"
	fi
	remove_label
	exit 1
}

check_prereqs() {
	local merged="$(echo $GITHUB_CONTEXT | jq --raw-output .event.pull_request.merged)"
	if [ "$merged" != false ]
	then
		bail_out "PR is already merged"
	fi

	local mergeable="$(echo $GITHUB_CONTEXT | jq --raw-output .event.pull_request.mergeable)"
	if [ "$mergeable" != true ]
	then
		bail_out "PR does not seem to be mergeable"
	fi

	local base_ref="$(echo $GITHUB_CONTEXT | jq --raw-output .base_ref)"
	if [ "$base_ref" != master ]
	then
		bail_out "PR is not based on master"
	fi
}

run_hooks() {
	# Run some pretend "hooks" that might commit something.

	date >> commit_log.txt
	echo "hooks run :)" >> commit_log.txt

	git config user.name "GitHub Actions Bot"
	git config user.email "<>"
	git add commit_log.txt

	local remote=$(echo $GITHUB_CONTEXT | jq --raw-output .repositoryUrl)
	git remote add target "$remote" # In case this was checked out via https.

	if git commit -m "run hooks"
	then
		git push target master || bail_out "unable to push to master branch"
	fi

	# TODO: Add a link to the pushed commit?
	add_comment "Pushed to master"
	remove_label
	close_issue
}

check_prereqs
run_hooks

