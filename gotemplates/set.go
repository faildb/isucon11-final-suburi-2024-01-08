package gotemplates

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/samber/lo"
)

func Set(c echo.Context) error {
	strIds := []string{"1", "2", "3"}

	var loErrs []error
	idsSet := lo.SliceToMap[string, int, struct{}](strIds, func(strId string) (int, struct{}) {
		id, err := strconv.ParseInt(strId, 10, 64)
		if err != nil {
			c.Logger().Error(err)
			loErrs = append(loErrs, err)
		}
		return int(id), struct{}{}
	})

	if len(loErrs) != 0 {
		c.Logger().Error(
			strings.Join(
				lo.Map(loErrs, func(e error, idx int) string { return e.Error() }),
				", ",
			),
		)
		return c.NoContent(http.StatusInternalServerError)
	}
}
