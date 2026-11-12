package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/1mb-dev/markgo/internal/models"
)

// goalTestArticles: two amount-bearing published posts, one draft with an
// amount (must not count), one published without.
func goalTestArticles() []*models.Article {
	now := time.Now()
	return []*models.Article{
		{Slug: "with-amount-a", Title: "A", Date: now, Amount: 10.5},
		{Slug: "with-amount-b", Title: "B", Date: now.Add(-24 * time.Hour), Amount: 20},
		{Slug: "draft-with-amount", Title: "D", Date: now.Add(-48 * time.Hour), Amount: 100, Draft: true},
		{Slug: "no-amount", Title: "N", Date: now.Add(-72 * time.Hour)},
	}
}

func TestGoal_Handler(t *testing.T) {
	t.Run("lists only published amount posts, sum excludes drafts", func(t *testing.T) {
		base, svc := createTestBase()
		svc.articles = goalTestArticles()

		mockTpl := requireMockTemplateService(t, base)
		mockTpl.LastData = nil

		router := gin.New()
		router.GET("/goal", NewPostHandler(base, svc).Goal)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/goal", http.NoBody))

		require.Equal(t, http.StatusOK, w.Code)
		require.NotNil(t, mockTpl.LastData)
		assert.Equal(t, "goal", mockTpl.LastData["template"])
		assert.Equal(t, "/goal", mockTpl.LastData["canonicalPath"])

		articles, ok := mockTpl.LastData["articles"].([]*models.Article)
		require.True(t, ok)
		require.Len(t, articles, 2, "draft and amountless posts must be filtered out")

		goal, ok := mockTpl.LastData["goal"].(*models.GoalProgress)
		require.True(t, ok)
		assert.Equal(t, 30.5, goal.Sum)
	})

	t.Run("pagination follows page param", func(t *testing.T) {
		base, svc := createTestBase()
		svc.articles = goalTestArticles()
		base.config.Blog.PostsPerPage = 1

		mockTpl := requireMockTemplateService(t, base)
		mockTpl.LastData = nil

		router := gin.New()
		router.GET("/goal", NewPostHandler(base, svc).Goal)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/goal?page=2", http.NoBody))

		require.Equal(t, http.StatusOK, w.Code)
		pagination, ok := mockTpl.LastData["pagination"].(*models.Pagination)
		require.True(t, ok)
		assert.Equal(t, 2, pagination.CurrentPage)
		assert.Equal(t, 2, pagination.TotalPages)

		articles, ok := mockTpl.LastData["articles"].([]*models.Article)
		require.True(t, ok)
		require.Len(t, articles, 1)
	})

	t.Run("goal hidden when BLOG_GOAL_TOTAL unset", func(t *testing.T) {
		base, svc := createTestBase()
		svc.articles = goalTestArticles()

		mockTpl := requireMockTemplateService(t, base)
		mockTpl.LastData = nil

		router := gin.New()
		router.GET("/goal", NewPostHandler(base, svc).Goal)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/goal", http.NoBody))

		require.Equal(t, http.StatusOK, w.Code)
		goal, ok := mockTpl.LastData["goal"].(*models.GoalProgress)
		require.True(t, ok)
		assert.False(t, goal.Show, "zero total must hide the bar")
	})
}

// TestWriting_GoalData — the /writing page carries the same goal progress
// data (rendered beside the "Writing" title).
func TestWriting_GoalData(t *testing.T) {
	base, svc := createTestBase()
	svc.articles = goalTestArticles()
	base.config.Blog.GoalTotal = 61

	mockTpl := requireMockTemplateService(t, base)
	mockTpl.LastData = nil

	router := gin.New()
	router.GET("/writing", NewPostHandler(base, svc).Articles)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/writing", http.NoBody))

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, mockTpl.LastData)
	goal, ok := mockTpl.LastData["goal"].(*models.GoalProgress)
	require.True(t, ok)
	assert.Equal(t, 30.5, goal.Sum)
	assert.True(t, goal.Show)
}

func TestFeed_GoalData(t *testing.T) {
	base, svc := createTestBase()
	svc.articles = goalTestArticles()
	base.config.Blog.GoalTotal = 61

	mockTpl := requireMockTemplateService(t, base)
	mockTpl.LastData = nil

	router := gin.New()
	router.GET("/", NewFeedHandler(base, svc).Home)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", http.NoBody))

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, mockTpl.LastData)
	goal, ok := mockTpl.LastData["goal"].(*models.GoalProgress)
	require.True(t, ok)
	assert.Equal(t, 30.5, goal.Sum)
	assert.Equal(t, 61.0, goal.Total)
	assert.InDelta(t, 50.0, goal.Percent, 0.01)
	assert.True(t, goal.Show)
}

func TestNewGoalProgress(t *testing.T) {
	t.Run("zero total hides", func(t *testing.T) {
		g := models.NewGoalProgress(10, 0)
		assert.False(t, g.Show)
	})
	t.Run("negative total hides", func(t *testing.T) {
		g := models.NewGoalProgress(10, -5)
		assert.False(t, g.Show)
	})
	t.Run("percent clamped at 100", func(t *testing.T) {
		g := models.NewGoalProgress(200, 100)
		assert.True(t, g.Show)
		assert.Equal(t, 100.0, g.Percent)
		assert.Equal(t, 200.0, g.Sum, "sum keeps the real number for the tooltip")
	})
	t.Run("zero sum still shows empty bar", func(t *testing.T) {
		g := models.NewGoalProgress(0, 100)
		assert.True(t, g.Show)
		assert.Equal(t, 0.0, g.Percent)
	})
	t.Run("percent rounded to two decimals", func(t *testing.T) {
		g := models.NewGoalProgress(1, 3)
		assert.Equal(t, 33.33, g.Percent)
	})
}
