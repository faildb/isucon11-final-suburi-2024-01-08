package gotemplates

import (
	"encoding/json"
	"fmt"
	"github.com/cespare/xxhash/v2"
	"github.com/labstack/echo/v4"
	"net/http"
	"strings"
)

type Resp struct {
	HogeData []int  `json:"hoge_data"`
	FugaStr  string `json:"fuga_str"`
}

// Handle304NotModified - 帯域が狭い環境では、レスポンスのサイズを小さくすることで、レスポンスの転送にかかる時間を短縮できる
func Handle304NotModified(c echo.Context) error {

	// レスポンスを計算する。redisのキャッシュなどは使っても使わなくてもよい
	resp := Resp{
		HogeData: []int{1, 2, 3},
		FugaStr:  "fuga",
	}

	// レスポンスをJSONに変換
	respBlob, err := json.Marshal(resp)
	if err != nil {
		c.Logger().Error(fmt.Errorf("failed to marshal json: %w", err))
		return c.NoContent(http.StatusInternalServerError)
	}

	// レスポンスからETagとLast-Modifiedを生成
	etag := GenerateETag(respBlob)

	// リクエストヘッダのETagと今生成したETagが一致するか比較し、一致したら304を返す
	if ifNoneMatch := c.Request().Header.Get("If-None-Match"); ifNoneMatch != "" {
		if strings.Contains(ifNoneMatch, etag) {
			//if ifNoneMatch == etag || ifNoneMatch == fmt.Sprintf("W/%s", etag) {
			return c.NoContent(http.StatusNotModified)
		}
	}

	// 一致しなかったら新しいETagをレスポンスヘッダにセットしてレスポンスする
	c.Response().Header().Set("ETag", etag)
	return c.JSONBlob(http.StatusOK, respBlob)
}

var digest = xxhash.New()

// GenerateETag はレスポンスのETagを生成する
// in: "",     out: `"ef46db3751d8e999"`
// in: "hoge", out: `"4e949f722df978d8"`
func GenerateETag(b []byte) string {
	digest.Reset()
	_, _ = digest.Write(b)
	// ETagは二重引用符で囲わないとNginxでgzip圧縮されるときに削除される
	// 二重引用符で囲うとgzip圧縮される際にweakenされる（W/が先頭につく）
	return fmt.Sprintf(`"%x"`, digest.Sum64())
}
