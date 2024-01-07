.PHONY: *

help:  ## makeのヘルプを表示
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

list-services: ## systemdのサービス一覧を表示
	sudo systemctl list-units --type=service

list-listening-ports: ## LISTEN状態のポートをプロセス名とともに表示
	sudo ss -ntlp

gzip-static-files: ## 静的ファイルをgzip圧縮するコマンドを出力
	@echo '`find /path/to/directory -type f ! -name "*.gz" -exec gzip -k {} \;` を静的ファイルのディレクトリ実行するとよい'
