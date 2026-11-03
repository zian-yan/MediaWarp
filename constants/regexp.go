package constants

import "regexp"

type CacheRegexps struct {
	Image    *regexp.Regexp
	Subtitle *regexp.Regexp
}

type EmbyRegexps struct {
	Router RouterRegexps
	Others OthersRegexps
	Cache  CacheRegexps
}

type RouterRegexps struct {
	VideosHandler        *regexp.Regexp
	ModifyBaseHtmlPlayer *regexp.Regexp
	ModifyPlaybackInfo   *regexp.Regexp
}

type OthersRegexps struct {
	VideoRedirectReg *regexp.Regexp
}

var EmbyRegexp = &EmbyRegexps{
	Router: RouterRegexps{
		VideosHandler:        regexp.MustCompile(`(?i)^(/emby)?/Videos/\d+/(stream|original)(\.\w+)?$`),
		ModifyBaseHtmlPlayer: regexp.MustCompile(`(?i)^/web/modules/htmlvideoplayer/basehtmlplayer.js$`),
		ModifyPlaybackInfo:   regexp.MustCompile(`(?i)^(/emby)?/Items/\d+/PlaybackInfo$`),
	},
	Others: OthersRegexps{
		VideoRedirectReg: regexp.MustCompile(`(?i)^(/emby)?/videos/(.*)/stream/(.*)`),
	},
	Cache: CacheRegexps{
		Image:    regexp.MustCompile(`(?i)^(/emby)?/Items/\d+/Images(/.*)?$`),
		Subtitle: regexp.MustCompile(`(?i)/Videos/(.*)/Subtitles/(.*)/Stream\.(ass|ssa|srt|)?$`),
	},
}

type JellyfinRouterRegexps struct {
	VideosHandler      *regexp.Regexp
	ModifyPlaybackInfo *regexp.Regexp
}
type JellyfinRegexps struct {
	Router JellyfinRouterRegexps
	Cache  CacheRegexps
}

var JellyfinRegexp = &JellyfinRegexps{
	Router: JellyfinRouterRegexps{
		VideosHandler:      regexp.MustCompile(`/Videos/[\w-]+/(stream|original)(\.\w+)?$`),
		ModifyPlaybackInfo: regexp.MustCompile(`^/Items/\w+/PlaybackInfo$`),
	},
	Cache: CacheRegexps{
		Image: regexp.MustCompile(`(?i)/Items/\w+/Images(/.*)?$`),

		Subtitle: regexp.MustCompile(`(?i)/Videos/(.*)/Subtitles/(.*)/Stream\.(ass|ssa|srt|)?$`),
	},
}
