vcl 4.1;

import directors;
import std;

# Default backend definition. Set this to point to your content server.
# backend default {
#     .host = "127.0.0.1";
#     .port = "8080";
# }

backend isu1 {
    .host = "192.168.0.11";
    .port = "3000";
}

# backend isu2 {
#     .host = "192.168.0.12";
#     .port = "8080";
# }

# backend isu3 {
#     .host = "192.168.0.13";
#     .port = "3000";
# }

sub vcl_init {
    # ラウンドロビンでリクエストを送る
    # new bar = directors.round_robin();
    # bar.add_backend(isu1);
    # bar.add_backend(isu2);

    # 重み付けでリクエストを送る
    new vdir = directors.random();
    # bar.add_backend(isu2);
    # 2/3 -> isu1, 1/3 -> isu2.
    # vdir.add_backend(isu1, 10.0);
    # vdir.add_backend(isu2, 5.0);

    vdir.add_backend(isu1, 4.0);
    # vdir.add_backend(isu3, 6.0);
}

# acl purge {
#     "localhost";
#     "192.168.0.0"/24;
# }

sub vcl_recv {
    # 重み付けを使うときは以下を記述しないと動かない
    set req.backend_hint = vdir.backend();

    # 特定パスだけは別のバックエンドに送る
    #    if (req.url ~ "^/java/") {
    #        set req.backend_hint = isu1;
    #    } else {
    #        set req.backend_hint = isu2;
    #    }


    # 特定のURLをキャッシュしないようにする
    if (req.url ~ "^/nocache") {
        return (pass);
    }

    # クッキーを削除してキャッシュを可能にする
    if (req.url ~ "^/cacheable") {
        unset req.http.cookie;
    }

    # ハッシュ以降を削除。キャッシュには必要ない
    if (req.url ~ "\#") {
        set req.url = regsub(req.url, "\#.*$", "");
    }

    # 末尾の?を削除。キャッシュには必要ない
    if (req.url ~ "\?$") {
        set req.url = regsub(req.url, "\?$", "");
    }

    # ban
    # ↓のコマンドでhit missの流れが見える
    # sudo varnishncsa -F '%m %U%q %{Varnish:hitmiss}x'
    if (req.method == "BAN") {
        # Same ACL check as above:
        # アクセス制限はしない（実運用だと死ぬ）
        # if (!client.ip ~ purge) {
        #         return(synth(403, "Not allowed."));
        # }
        std.log("BAN HOST: ~ " + req.http.X-Host-Invalidation-Pattern);
        std.log("BAN URL: ~ " + req.http.X-Url-Invalidation-Pattern);
        if (std.ban("obj.http.x-host ~ " + req.http.X-Host-Invalidation-Pattern + " && obj.http.x-url ~ " + req.http.X-Url-Invalidation-Pattern)) {
            return(synth(200, "Ban added"));
        } else {
            return(synth(400, std.ban_error()));
        }
    }


    # ----------------------------  isucon のURL  ----------------------------

    # e.POST("/api/admin/tenants/add", tenantsAddHandler)
 	if (req.url ~ "^/api/admin/tenants/add" && req.method == "POST") {
        # set req.backend_hint = isu1;
        return (pass);
    }
	# e.GET("/api/admin/tenants/billing", tenantsBillingHandler)
	if (req.url ~ "^/api/admin/tenants/billing" && req.method == "GET") {
        # set req.backend_hint = isu1;
        return (hash);
    }

	# テナント管理者向けAPI - 参加者追加、一覧、失格
	# e.GET("/api/organizer/players", playersListHandler)
	if (req.url ~ "^/api/organizer/players" && req.method == "GET") {
        # set req.backend_hint = isu1;
        return (pass);
    }
	# e.POST("/api/organizer/players/add", playersAddHandler)
	if (req.url ~ "^/api/organizer/players/add" && req.method == "POST") {
        # set req.backend_hint = isu1;
        return (pass);
    }
	# e.POST("/api/organizer/player/:player_id/disqualified", playerDisqualifiedHandler)
	if (req.url ~ "^/api/organizer/player/" && req.method == "POST") {
        # set req.backend_hint = isu1;
        return (pass);
    }

	// テナント管理者向けAPI - 大会管理
	# e.POST("/api/organizer/competitions/add", competitionsAddHandler)
	if (req.url ~ "^/api/organizer/competitions/add" && req.method == "POST") {
        # set req.backend_hint = isu1;
        return (pass);
    }
	# e.POST("/api/organizer/competition/:competition_id/finish", competitionFinishHandler)
	if (req.url ~ "^/api/organizer/competition/[a-zA-Z0-9-_]+/finish" && req.method == "POST") {
        # set req.backend_hint = isu1;
        return (pass);
    }
	# e.POST("/api/organizer/competition/:competition_id/score", competitionScoreHandler)
	if (req.url ~ "^/api/organizer/competition/[a-zA-Z0-9-_]+/score" && req.method == "POST") {
        # set req.backend_hint = isu1;
        return (pass);
    }
	# e.GET("/api/organizer/billing", billingHandler)
	if (req.url ~ "^/api/organizer/billing" && req.method == "GET") {
        # set req.backend_hint = isu1;
        return (hash);
    }
	# e.GET("/api/organizer/competitions", organizerCompetitionsHandler)
	if (req.url ~ "^/api/organizer/competitions" && req.method == "GET") {
        # set req.backend_hint = isu1;
        return (pass);
    }

	# 参加者向けAPI
	# e.GET("/api/player/player/:player_id", playerHandler)
	if (req.url ~ "^/api/player/player/[a-zA-Z0-9-_]+" && req.method == "GET") {
        # set req.backend_hint = isu1;
        return (pass);
    }
	# e.GET("/api/player/competition/:competition_id/ranking", competitionRankingHandler)
	if (req.url ~ "^/api/player/competition/[a-zA-Z0-9-_]+/ranking" && req.method == "GET") {
        # set req.backend_hint = isu1;
        return (pass);
    }
	# e.GET("/api/player/competitions", playerCompetitionsHandler)
	if (req.url ~ "^/api/player/competitions" && req.method == "GET") {
        # set req.backend_hint = isu1;
        return (pass);
    }

	// 全ロール及び未認証でも使えるhandler
	# e.GET("/api/me", meHandler)
	if (req.url ~ "^/api/me" && req.method == "GET") {
        # set req.backend_hint = isu1;
        return (pass);
    }

	// ベンチマーカー向けAPI
	# e.POST("/initialize", initializeHandler)
    if (req.url ~ "^/initialize" && req.method == "POST") {
        # set req.backend_hint = isu1;
        return (pass);
    }
    # ----------------------------  isucon のURL  ----------------------------
}

sub vcl_builtin_recv {
    std.log("vcl_builtin_recv");
	call vcl_req_host;
	call vcl_req_method;
}


sub vcl_req_authorization {
    # Authorization Headerがあってもキャッシュしたい場合はコメントアウト
    # if (req.http.Authorization) {
    #      # Not cacheable by default.
    #      return (pass);
    # }
}

sub vcl_req_cookie {
    # cookieがあってもキャッシュしたい場合はreturn (hash); する
    std.log("vcl_req_cookie");
    return (hash);
    # cookieがあってもキャッシュしたい場合はコメントアウト
    #    if (req.http.Cookie) {
    #         # Risky to cache by default.
    #         return (pass);
    #    }
}

sub vcl_hash {
  # vcl_recvのあとに呼ばれる
  # cacheのkeyを決める

  # デフォルトでは パス + クエリパラメータ でキャッシュする
  hash_data(req.url);

  # Host, サーバーIPでキャッシュ内容を変えたい場合は以下を追加
  if (req.http.host) {
    hash_data(req.http.host);
  } else {
    hash_data(server.ip);
  }

  # Cookieでキャッシュ内容を変えたい場合は以下を追加
  #  if (req.http.Cookie) {
  #    hash_data(req.http.Cookie);
  #  }

  # http httpsでキャッシュ内容を変えたい場合は以下を追加
  # if (req.http.X-Forwarded-Proto) {
  #   hash_data(req.http.X-Forwarded-Proto);
  # }
  return (lookup);
}

sub vcl_backend_response {
    # BANで消すときの目印に使う
    set beresp.http.x-host = bereq.http.host;
    set beresp.http.x-url = bereq.url;

    # レスポンスのキャッシュ時間を設定
    set beresp.ttl = 10s;

    # キャッシュされたオブジェクトの有効期限後も、一定時間レスポンスを提供する
    # この間にバックエンドからのレスポンスが返ってきたら、それをキャッシュする
    # stale-while-revalidate を実現する
    set beresp.grace = 10s;

    # キャッシュされたオブジェクトの有効期限後に、varnishがキャッシュを保持する時間
    # If-Modified-Since: and/or Ìf-None-Match がある場合に 304 Not Modifiedを実現するために設定
    set beresp.keep = 30s;


    # 特定のパスに対するTTLの設定
    # if (bereq.url ~ "^/short-term") {
    #     set beresp.ttl = 0.5s;
    # } elseif (bereq.url ~ "^/long-term") {
    #     set beresp.ttl = 10m;
    # } else {
    #     set beresp.ttl = 1s;
    # }

    # ----------------------------  isucon のURL  ----------------------------
	if (bereq.url ~ "^/api/admin/tenants/billing" && bereq.method == "GET") {
        set beresp.ttl = 10s;
        set beresp.grace = 0s;
        set beresp.keep = 30s;
	}
	if (bereq.url ~ "^/api/organizer/billing" && bereq.method == "GET") {
        set beresp.ttl = 10s;
        set beresp.grace = 0s;
        set beresp.keep = 30s;
	}
    # ----------------------------  isucon のURL  ----------------------------

    # varnishでgzipしたい場合は以下を追加
    if (beresp.http.content-type ~ "text" || beresp.http.content-type ~ "json" ) {
        set beresp.do_gzip = true;
    }
}


sub vcl_deliver {
    # BANで消すときの目印なので、レスポンスには含めなくてよい
    unset resp.http.x-host;
    unset resp.http.x-url;

    # レスポンスヘッダーにキャッシュの状態を追加
    if (obj.hits > 0) {
        set resp.http.X-Cache = "HIT";
    } else {
        set resp.http.X-Cache = "MISS";
    }

    # 回によってはCache-Control: privateを付与しないとfailすることがあるので追加。基本varnish以降cdnを挟まないのでつけておいてもデメリットはない。
    # 例. https://github.com/isucon/isucon12-qualify/blob/main/webapp/specification.md#response%E3%81%AEhttp-header
    if (resp.http.Cache-Control) {
        set resp.http.Cache-Control = resp.http.Cache-Control + ", private";
    } else {
        set resp.http.Cache-Control = "private";
    }
}
