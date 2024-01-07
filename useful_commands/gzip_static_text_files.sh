#!/usr/bin/env bash

set -eux

find . -type f \( -name '*.html' -o -name '*.css' -o -name '*.js' \) -exec sh -c 'gzip -vc "{}" > "{}.gz"' \;


# 例
# ```console
# $ find . -type f \( -name '*.html' -o -name '*.css' -o -name '*.js' \) -exec sh -c 'gzip -vc "{}" > "{}.gz"' \;
# ./js/main.js:	 58.3%
# ./js/timeago.min.js:	 45.4%
# ./css/style.css:	 69.9%
# ```