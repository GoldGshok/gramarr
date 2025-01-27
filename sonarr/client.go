package sonarr

import (
	"fmt"
	"github.com/go-resty/resty/v2"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	apiRgx = regexp.MustCompile(`[a-z0-9]{32}`)
)

func NewClient(c Config) (*Client, error) {
	if c.Hostname == "" {
		return nil, fmt.Errorf("hostname is empty")
	}

	if match := apiRgx.MatchString(c.APIKey); !match {
		return nil, fmt.Errorf("api key is invalid format: %s", c.APIKey)
	}

	baseURL := createApiURL(c)

	r := resty.New()
	r.SetBaseURL(baseURL)
	r.SetHeader("Accept", "application/json")
	r.SetQueryParam("apikey", c.APIKey)
	if c.Username != "" && c.Password != "" {
		r.SetBasicAuth(c.Username, c.Password)
	}

	client := Client{
		apiKey:     c.APIKey,
		maxResults: c.MaxResults,
		username:   c.Username,
		password:   c.Password,
		baseURL:    baseURL,
		client:     r,
	}
	return &client, nil
}

func createApiURL(c Config) string {
	c.Hostname = strings.TrimPrefix(c.Hostname, "http://")
	c.Hostname = strings.TrimPrefix(c.Hostname, "https://")
	c.URLBase = strings.TrimPrefix(c.URLBase, "/")

	u := url.URL{}
	if c.SSL {
		u.Scheme = "https"
	} else {
		u.Scheme = "http"
	}

	if c.Port == 80 {
		u.Host = c.Hostname
	} else {
		u.Host = fmt.Sprintf("%s:%d", c.Hostname, c.Port)
	}

	u.Path = "/api/v3"
	if c.URLBase != "" {
		u.Path = fmt.Sprintf("%s%s", c.URLBase, u.Path)
	}

	return u.String()
}

type Client struct {
	apiKey     string
	username   string
	password   string
	baseURL    string
	maxResults int
	client     *resty.Client
}

func (c *Client) DeleteTVShow(tvShowId int) (err error) {
	_, err = c.client.R().SetQueryParam("deleteFiles", "true").Delete("series/" + strconv.Itoa(tvShowId))
	return
}

func (c *Client) SearchTVShow(tvdbId int) (movie TVShow, err error) {
	resp, err := c.client.R().SetQueryParam("term", "tvdb:"+strconv.Itoa(tvdbId)).SetResult(TVShow{}).Get("series/lookup")
	if err != nil {
		return
	}
	movie = *resp.Result().(*TVShow)
	return
}

func (c *Client) SearchTVShows(term string) ([]TVShow, error) {
	resp, err := c.client.R().SetQueryParam("term", term).SetResult([]TVShow{}).Get("series/lookup")
	if err != nil {
		return nil, err
	}

	tvShows := *resp.Result().(*[]TVShow)
	if len(tvShows) > c.maxResults {
		tvShows = tvShows[:c.maxResults]
	}
	return tvShows, nil
}

func (c *Client) GetFolders() ([]Folder, error) {
	resp, err := c.client.R().SetResult([]Folder{}).Get("rootfolder")
	if err != nil {
		return nil, err
	}

	folders := *resp.Result().(*[]Folder)
	return folders, nil
}

func (c *Client) GetProfile(prfl string) ([]Profile, error) {
	resp, err := c.client.R().SetResult([]Profile{}).Get(prfl)
	if err != nil {
		return nil, err
	}
	profile := *resp.Result().(*[]Profile)

	return profile, nil
}

func (c *Client) GetSystemStatus() (SystemStatus, error) {
	var systemStatus SystemStatus

	resp, err := c.client.R().SetResult(SystemStatus{}).Get("/system/status")
	if err != nil {
		return systemStatus, err
	}
	systemStatus = *resp.Result().(*SystemStatus)

	return systemStatus, nil
}

func (c *Client) AddTVShow(m TVShow, languageProfile int, qualityProfile int, path string, requester string) (tvShow TVShow, err error) {

	request := AddTVShowRequest{
		Title:             m.Title,
		TitleSlug:         m.TitleSlug,
		Images:            m.Images,
		QualityProfileID:  qualityProfile,
		LanguageProfileID: languageProfile,
		TVDBID:            m.TvdbID,
		RootFolderPath:    path,
		Monitored:         true,
		SeasonFolder:      true,
		Year:              m.Year,
		Seasons:           m.Seasons,
		AddOptions:        AddTVShowOptions{SearchForMissingEpisodes: true},
	}

	tag, err := c.GetTagByLabel(requester, true)
	if err == nil {
		request.Tags = []int{tag.Id}
	}

	resp, err := c.client.R().SetBody(request).SetResult(TVShow{}).Post("series")
	if err != nil {
		return
	}

	tvShow = *resp.Result().(*TVShow)
	return
}

func (c *Client) GetTagByLabel(label string, createNew bool) (tvShowTag TVShowTag, err error) {
	tags, err := c.GetTags()
	if err != nil {
		return
	}
	for _, tag := range tags {
		if strings.EqualFold(strings.TrimSpace(label), strings.TrimSpace(tag.Label)) {
			tvShowTag = tag
		}
	}
	if createNew && tvShowTag.Id == 0 {
		tvShowTag, err = c.CreateTag(strings.TrimSpace(label))
	}
	return
}

func (c *Client) GetTagById(id int) (tvShowTag TVShowTag, err error) {
	tags, err := c.GetTags()
	if err != nil {
		return
	}
	for _, tag := range tags {
		if id == tag.Id {
			tvShowTag = tag
			return
		}
	}
	return
}

func (c *Client) GetTags() (tags []TVShowTag, err error) {
	resp, err := c.client.R().SetResult([]TVShowTag{}).Get("tag")
	if err != nil {
		return
	}
	tags = *resp.Result().(*[]TVShowTag)
	return
}

func (c *Client) CreateTag(label string) (tag TVShowTag, err error) {
	label = strings.TrimSpace(label)
	resp, err := c.client.R().SetBody(TVShowTag{Label: label}).SetResult(TVShowTag{}).Post("tag")
	if err != nil {
		return
	}
	tag = *resp.Result().(*TVShowTag)
	return
}

func (c *Client) GetRequesterList(tvShow TVShow) (requester []string) {
	for _, tagId := range tvShow.Tags {
		tag, err := c.GetTagById(tagId)
		if err == nil {
			requester = append(requester, strings.Title(tag.Label))
		}
	}
	return
}

func (c *Client) AddRequester(t TVShow, requester string) (TVShow, error) {
	tag, err := c.GetTagByLabel(requester, true)
	if err != nil {
		return t, err
	}
	t.Tags = append(t.Tags, tag.Id)
	return c.UpdateTVShow(t)
}

func (c *Client) RemoveRequester(t TVShow, requester string) (TVShow, error) {
	tag, err := c.GetTagByLabel(requester, true)
	if err != nil {
		return t, err
	}
	var filteredTags []int
	for i := range t.Tags {
		if t.Tags[i] != tag.Id {
			filteredTags = append(filteredTags, t.Tags[i])
		}
	}
	t.Tags = filteredTags
	return c.UpdateTVShow(t)
}

func (c *Client) UpdateTVShow(t TVShow) (tvShow TVShow, err error) {
	resp, err := c.client.R().SetBody(t).SetResult(TVShow{}).Put("series")
	if err != nil {
		return
	}
	tvShow = *resp.Result().(*TVShow)
	return
}

func (c *Client) GetTVShowsByRequester(requester string) (tvShows []TVShow, err error) {
	allTVShows, err := c.GetTVShows()
	if err != nil {
		return
	}
	for _, tvShow := range allTVShows {
		for _, t := range tvShow.Tags {
			tag, _ := c.GetTagById(t)
			if strings.Trim(requester, " ") == strings.Trim(tag.Label, " ") {
				tvShows = append(tvShows, tvShow)
			}
		}
	}
	return
}

func (c *Client) GetTVShowsByFolder(folder Folder) (tvShows []TVShow, err error) {
	allTVShows, err := c.GetTVShows()
	if err != nil {
		return
	}
	for _, tvShow := range allTVShows {
		if strings.HasPrefix(tvShow.Path, folder.Path) {
			tvShows = append(tvShows, tvShow)
		}
	}
	return
}

func (c *Client) GetTVShows() (tvShows []TVShow, err error) {
	resp, err := c.client.R().SetResult([]TVShow{}).Get("series")
	if err != nil {
		return
	}
	tvShows = *resp.Result().(*[]TVShow)
	return
}

func (c *Client) GetMonitoredTVShows() (tvShows []TVShow, err error) {
	allTVShows, _ := c.GetTVShows()
	if err != nil {
		return
	}
	for _, tvShow := range allTVShows {
		if tvShow.Monitored {
			tvShows = append(tvShows, tvShow)
		}
	}
	return
}

func (c *Client) GetTVShow(tvShowId int) (tvShow TVShow, err error) {
	resp, err := c.client.R().SetResult(TVShow{}).Get("series/" + strconv.Itoa(tvShowId))
	if err != nil {
		return
	}
	tvShow = *resp.Result().(*TVShow)
	return
}

func (c *Client) GetPosterURL(tvShow TVShow) string {
	for _, image := range tvShow.Images {
		if image.CoverType == "poster" {
			return image.RemoteURL
		}
	}
	return ""
}
