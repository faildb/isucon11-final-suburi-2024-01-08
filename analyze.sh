#!/usr/bin/env bash

set -eu
cd `dirname $0`

################################################################################
echo "# Analyze"
################################################################################

# read env
# 計測用自作env
. /tmp/prepared_env

# isucon serviceで使うenv
. ./env.sh

result_dir=$HOME/result
mkdir -p ${result_dir}

# journal log
sudo journalctl -e -nall -ocat -u isucon.go.service --since="${prepared_time}" > "${app_journal_log}"
sudo journalctl -e -nall -ocat -u openresty.service --since="${prepared_time}" > "${nginx_journal_log}"

# alp
# ALPM="/int/\d+,/uuid/[A-Za-z0-9_]+,/6digits/[a-z0-9]{6}"
#ALPM="/@.+,/posts/\d+,/image/\d+.(jpg|png|gif),/posts?max_created_at.*$"
#ALPM="/api/courses/[a-zA-Z0-9]+$,/api/courses/[a-zA-Z0-9]+/status,/api/courses/[a-zA-Z0-9]+/classes,/api/courses/[a-zA-Z0-9]+/classes/[a-zA-Z0-9]+/assignments,/api/courses/[a-zA-Z0-9]+/classes/[a-zA-Z0-9]+/assignments/scores,/api/courses/[a-zA-Z0-9]+/classes/[a-zA-Z0-9]+/assignments/export,/api/announcements/[a-zA-Z0-9]+$"
#ALPM="/initialize,/api/admin/clarifications,/api/admin/clarifications/\d,/api/session,/api/audience/teams,/api/audience/dashboard,/api/registration/session,/api/registration/team,/api/registration/contestant,/api/registration,/api/registration,/api/contestant/benchmark_jobs,/api/contestant/benchmark_jobs/\d,/api/contestant/clarifications,/api/contestant/clarifications,/api/contestant/dashboard,/api/contestant/notifications,/api/contestant/push_subscriptions,/api/contestant/push_subscriptions,/api/signup,/api/login,/api/logout"
ALPM="/api/organizer/player/[a-zA-Z0-9-_]+/disqualified,/api/organizer/competition/[a-zA-Z0-9-_]+/finish,/api/organizer/competition/[a-zA-Z0-9-_]+/score,/api/player/player/[a-zA-Z0-9-_]+,/api/player/competition/[a-zA-Z0-9-_]+/ranking"

echo -e "\n# ALPの集計結果"

OUTFORMT=count,1xx,2xx,3xx,4xx,5xx,method,uri,min,max,sum,avg,p95,min_body,max_body,avg_body
touch ${result_dir}/alp.md
cp ${result_dir}/alp.md ${result_dir}/alp.md.prev
alp json --file=${nginx_access_log} \
  --nosave-pos \
  --sort sum \
  --reverse \
  --output ${OUTFORMT} \
  --format markdown \
  --matching-groups ${ALPM}  \
  | tee ${result_dir}/alp.md

# OUTFORMT=count,uri_method_status,min,max,sum,avg,p95,trace_id_sample
# touch ${result_dir}/alp_trace.txt
# cp ${result_dir}/alp_trace.txt ${result_dir}/alp_trace.txt.prev
# alp-trace json --file=${nginx_access_log} \
#   --nosave-pos \
#   --sort sum \
#   --reverse \
#   --output ${OUTFORMT} \
#   --format pretty \
#   --matching-groups ${ALPM}  \
#   --trace \
#   | tee ${result_dir}/alp_trace.txt


# mysqlowquery
# sudo mysqldumpslow -s t ${mysql_slow_log} > ${result_dir}/mysqld-slow.txt

# touch ${result_dir}/pt-query-digest.txt
# cp ${result_dir}/pt-query-digest.txt ${result_dir}/pt-query-digest.txt.prev
# sudo chmod 755 `dirname ${mysql_slow_log}`
# sudo chmod 644 ${mysql_slow_log}
# pt-query-digest  --progress percentage,5 --explain "h=${DB_HOST},u=${DB_USER},p=${DB_PASS},D=${DB_DATABASE}" ${mysql_slow_log} > ${result_dir}/pt-query-digest.txt
# pt-query-digest ${mysql_slow_log} > ${result_dir}/pt-query-digest.txt


echo -e "\n# app 500エラー周辺のログ（あれば）"
touch log/app/5xx_journal.log
cp log/app/5xx_journal.log log/app/5xx_journal.log.prev
cat log/app/journal.log | grep -B3 -A3 -P '"status":5\d\d' | tee log/app/5xx_journal.log || true

echo -e "\n# nginx 500エラーのログ（あれば）"
touch log/nginx/5xx_access.log
cp log/nginx/5xx_access.log log/nginx/5xx_access.log.prev
cat log/nginx/access.log | grep -P '"status":"5\d\d"' | tee log/nginx/5xx_access.log || true

echo -e "\n# nginx のstatus code の集計"
touch ${result_dir}/status_code_nginx.txt
cat log/nginx/access.log | jq -r .status | sort | uniq -c | sort -nr | tee ${result_dir}/status_code_nginx.txt

echo -e "\n# app のstatus code の集計"
touch ${result_dir}/status_code_app.txt
cat log/app/journal.log  | grep -oP '{.*}' | jq -r .status | sort | uniq -c | sort -nr | tee ${result_dir}/status_code_app.txt

echo -e "\n# Varnishのキャッシュヒット率の集計"
touch ${result_dir}/varnish_cache_hit.txt
cat log/nginx/access.log | jq -r '.cache' | sort | uniq -c | sort -nr | tee ${result_dir}/cache_hit_varnish.txt

echo -e "\n# バックエンド(Goのプロセスキャッシュ・Redisなど)のキャッシュヒット率の集計（自分でCache-Statusレスポンスヘッダを付与しないと反映されない）"
touch ${result_dir}/varnish_cache_hit.txt
cat log/nginx/access.log | jq -r '.upstream_http_cache_status' | sort | uniq -c | sort -nr | tee ${result_dir}/cache_hit_backend.txt

echo -e "\n# nginxのAccept-Encodingの集計"
touch ${result_dir}/accept_encoding.txt
cat log/nginx/access.log | jq -r .accept_encoding | sort | uniq -c | sort -nr | tee ${result_dir}/accept_encoding.txt

echo -e "\n# nginxのUser-Agentの集計"
touch ${result_dir}/ua_nginx.txt
cat log/nginx/access.log | jq -r .ua | sort | uniq -c | sort -nr | tee ${result_dir}/ua_nginx.txt

echo -e "\n# appのUser-Agentの集計"
touch ${result_dir}/status_code_app.txt
cat log/app/journal.log  | grep -oP '{.*}' | jq -r .user_agent | sort | uniq -c | sort -nr | tee ${result_dir}/ua_app.txt

echo -e "\n# nginx のリクエストヘッダでif none match が空じゃない数の集計"
touch ${result_dir}/if_none_match_nginx.txt
non_empty_count=$(cat log/nginx/access.log  | jq -c 'select(.if_none_match != "")' | wc -l)
empty_count=$(cat log/nginx/access.log  | jq -c 'select(.if_none_match == null or .if_none_match == "")' | wc -l)
echo "non empty: ${non_empty_count}" | tee ${result_dir}/if_none_match_nginx.txt
echo "empty: ${empty_count}" | tee -a ${result_dir}/if_none_match_nginx.txt

echo -e "\n# app のリクエストヘッダでif none match が空じゃない数の集計"
touch ${result_dir}/if_none_match_app.txt
non_empty_count=$(cat log/app/journal.log  | grep -oP '{.*}' | jq -c 'select(.if_none_match != "")' | wc -l)
empty_count=$(cat log/app/journal.log  | grep -oP '{.*}' | jq -c 'select(.if_none_match == null or .if_none_match == "")' | wc -l)
echo "non empty: ${non_empty_count}" | tee ${result_dir}/if_none_match_app.txt
echo "empty: ${empty_count}" | tee -a ${result_dir}/if_none_match_app.txt


echo -e "\nOK"
