package provider

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func configure(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *Client {
	if req.ProviderData == nil {
		return nil
	}
	c, ok := req.ProviderData.(*Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
	}
	return c
}

func apiErr(d *diag.Diagnostics, what string, err error) {
	d.AddError("LakehouseBox: "+what, err.Error())
}

var durRe = regexp.MustCompile(`^\s*(\d+)\s*([smhd]?)\s*$`)

// seconds of a duration string (15m, 1h, 90s, 3600)
func seconds(v string) (int64, error) {
	m := durRe.FindStringSubmatch(v)
	if m == nil {
		return 0, fmt.Errorf("a duration is a number with s, m, h or d (15m, 1h), got %q", v)
	}
	n, _ := strconv.ParseInt(m[1], 10, 64)
	return n * map[string]int64{"": 1, "s": 1, "m": 60, "h": 3600, "d": 86400}[m[2]], nil
}
