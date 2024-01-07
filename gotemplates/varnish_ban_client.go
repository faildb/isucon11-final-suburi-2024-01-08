package gotemplates

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// TODO initalizeでvarnishのinvalidateを忘れないように

func VarnishBanAllCache(logger echo.Logger) error {
	return VarnishBan(logger, ".*", ".*")
}

var (
	VarnishHost      = GetEnv("VARNISH_HOST", "127.0.0.1")
	VarnishPort      = GetEnv("VARNISH_PORT", "6081")
	VarnishBanClient = http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        500,
			MaxIdleConnsPerHost: 200,
			IdleConnTimeout:     60 * time.Second,
		},
	}
)

// VarnishBan - VarnishにBANリクエストを投げる
// 即時でキャッシュを効かせないようにしたいときはVCLでgraceを0にすること
// RegexはPerl互換
func VarnishBan(logger echo.Logger, hostRegex, urlRegex string) error {
	reqUrl := fmt.Sprintf("http://%s:%s", VarnishHost, VarnishPort)
	req, err := http.NewRequest("BAN", reqUrl, nil)
	if err != nil {
		return fmt.Errorf("failed to create request, req url: %v, err: %w", reqUrl, err)
	}
	req.Header.Set("X-Host-Invalidation-Pattern", hostRegex)
	req.Header.Set("X-Url-Invalidation-Pattern", urlRegex)

	res, err := VarnishBanClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to do request, req url: %v, err: %w", reqUrl, err)
	}
	if logger != nil {
		logger.Debugf("VarnishBan: %v, status: %v", reqUrl, res.StatusCode)
	}
	defer res.Body.Close()
	// tcp connectionを再利用するためにはresponse bodyを読み切る必要がある
	_, err = io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body, req url: %v, err: %w", reqUrl, err)
	}
	return nil
}
