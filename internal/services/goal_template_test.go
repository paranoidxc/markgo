package services

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/1mb-dev/markgo/internal/config"
	"github.com/1mb-dev/markgo/internal/models"
)

func TestGoalTemplates_CSPCompatibleProgress(t *testing.T) {
	cfg := &config.Config{}
	service, err := NewTemplateService("", cfg)
	require.NoError(t, err)

	cases := []struct {
		name  string
		sum   float64
		total float64
		width string
	}{
		{name: "five percent", sum: 5, total: 100, width: "5%"},
		{name: "empty", sum: 0, total: 100, width: "0%"},
		{name: "negative clamped", sum: -5, total: 100, width: "0%"},
		{name: "fractional", sum: 1, total: 3, width: "33.33%"},
		{name: "complete", sum: 100, total: 100, width: "100%"},
		{name: "overflow clamped", sum: 150, total: 100, width: "100%"},
		{name: "disabled", sum: 5, total: 0},
	}

	for _, templateName := range []string{"feed-content", "articles-content", "goal-content"} {
		t.Run(templateName, func(t *testing.T) {
			for _, testCase := range cases {
				t.Run(testCase.name, func(t *testing.T) {
					output, err := service.RenderToString(templateName, map[string]any{
						"config":     cfg,
						"goal":       models.NewGoalProgress(testCase.sum, testCase.total),
						"pagination": &models.Pagination{},
					})
					require.NoError(t, err)
					assert.NotContains(t, output, "style=", "inline styles are blocked by the default CSP")
					if testCase.width == "" {
						assert.NotContains(t, output, "goal-bar-track")
						assert.NotContains(t, output, `role="tooltip"`)
						return
					}
					assert.Contains(t, output, `<svg class="goal-bar-track"`)
					assert.Contains(t, output, `role="progressbar"`)
					assert.Contains(t, output, `<rect class="goal-bar-fill" width="`+testCase.width+`"`)
					assert.NotContains(t, output, `title="无敌之日`)
					assert.Contains(t, output, `aria-describedby="goal-progress-tooltip"`)
					assert.Equal(t, 1, strings.Count(output, `id="goal-progress-tooltip"`))
					assert.Contains(t, output, `role="tooltip"`)
					assert.Contains(t, output, `<span class="goal-tooltip-percent">`+testCase.width+`</span>`)
					assert.Contains(t, output, fmt.Sprintf(`<strong class="goal-tooltip-current">¥%.2f</strong>`, testCase.sum))
					assert.Contains(t, output, fmt.Sprintf(`<strong>¥%.2f</strong>`, testCase.total))
					if templateName == "goal-content" {
						assert.Contains(t, output, `tabindex="0"`)
					}
				})
			}
		})
	}
}

func TestArticleTemplate_Amount(t *testing.T) {
	service, err := NewTemplateService("", &config.Config{})
	require.NoError(t, err)

	cases := []struct {
		name   string
		amount float64
		want   string
	}{
		{name: "positive", amount: 12.5, want: "¥12.50"},
		{name: "small", amount: 0.01, want: "¥0.01"},
		{name: "unset"},
		{name: "negative", amount: -5},
	}

	for _, articleType := range []string{"thought", "link", "article", "ama", "page"} {
		t.Run(articleType, func(t *testing.T) {
			for _, testCase := range cases {
				t.Run(testCase.name, func(t *testing.T) {
					output, err := service.RenderToString("article-content", map[string]any{
						"config": &config.Config{},
						"article": &models.Article{
							Type:   articleType,
							Title:  "Test post",
							Amount: testCase.amount,
						},
					})
					require.NoError(t, err)
					if testCase.want == "" {
						assert.NotContains(t, output, `class="article-amount"`)
						return
					}
					assert.Contains(t, output, `<span class="article-amount" title="Amount `+testCase.want+`">`+testCase.want+`</span>`)
				})
			}
		})
	}
}
