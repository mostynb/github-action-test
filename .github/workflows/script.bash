#!/bin/bash
set -euxo pipefail

echo "hello from bash script"

env

sudo apt-get install jq

remove_label() {
	curl -X DELETE -H "Accept: application/vnd.github.v3+json" \
		$(jq --raw-output github.event.pull_request._links.issue)/labels/merge-me
}

remove_label
