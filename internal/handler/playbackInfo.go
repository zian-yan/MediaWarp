package handler

import (
	"fmt"
	"strings"
	"time"

	"github.com/AkimioJR/MediaWarp/internal/config"
	"github.com/AkimioJR/MediaWarp/internal/logging"
	"github.com/AkimioJR/MediaWarp/utils"
)

// 修改 HTTPStrm 的 PlaybackInfo 响应
//
// 强制其支持直链播放；未开启 proxy 时禁用转码，并把直链指回 MediaWarp，由 MediaWarp 完成 302
func processHTTPStrmPlaybackInfo(jsonChain *utils.JsonChain, bsePath string, itemId string, id string, directStreamURL *string) {
	startTime := time.Now()
	defer func() {
		logging.Debugf("处理 HTTPStrm %s PlaybackInfo 耗时：%s", id, time.Since(startTime))
	}()

	var msgs []string

	jsonChain.Set(
		bsePath+"SupportsDirectPlay",
		true,
	)

	if !config.HTTPStrm.Proxy {
		jsonChain.Set(
			bsePath+"SupportsTranscoding",
			false,
		).Delete(
			bsePath + "TranscodingUrl",
		).Delete(
			bsePath + "TranscodingContainer",
		).Delete(
			bsePath + "TranscodingSubProtocol",
		).Delete(
			bsePath + "TrancodeLiveStartIndex",
		)
		msgs = append(msgs, "禁止转码行为")
	} else {
		msgs = append(msgs, "保持原有转码行为")
	}

	if directStreamURL != nil {
		// 日志中脱敏 api_key，避免密钥落盘
		msgs = append(msgs, fmt.Sprintf("原直链播放链接: %s", utils.MaskSensitiveURL(*directStreamURL)))
		apikeypair, err := utils.ResolveEmbyAPIKVPairs(directStreamURL)
		if err != nil {
			logging.Warning("解析API键值对失败：", err)
		}
		newDirectStreamURL := fmt.Sprintf("/Videos/%s/stream?MediaSourceId=%s&Static=true&%s", itemId, id, apikeypair)
		jsonChain.Set(
			bsePath+"DirectStreamUrl",
			newDirectStreamURL,
		)
		msgs = append(msgs, fmt.Sprintf("修改直链播放链接为: %s", utils.MaskSensitiveURL(newDirectStreamURL)))
	}
	logging.Infof("Media(id: %s) %s", id, strings.Join(msgs, ", "))
}
