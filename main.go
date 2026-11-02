package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/AkimioJR/MediaWarp/constants"
	"github.com/AkimioJR/MediaWarp/internal/config"
	"github.com/AkimioJR/MediaWarp/internal/handler"
	"github.com/AkimioJR/MediaWarp/internal/logging"
	"github.com/AkimioJR/MediaWarp/internal/router"
	"github.com/AkimioJR/MediaWarp/utils"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

var (
	isDebug     bool   // 开启调试模式
	showVersion bool   // 显示版本信息
	configPath  string // 配置文件路径
)

func init() {
	gin.SetMode(gin.ReleaseMode)

	flag.BoolVar(&showVersion, "version", false, "显示版本信息")
	flag.BoolVar(&isDebug, "debug", false, "是否启用调试模式")
	flag.StringVar(&configPath, "config", "config/config.yaml", "指定配置文件路径")
	flag.Parse()

	fmt.Println(constants.LOGO)
	fmt.Println(utils.Center(fmt.Sprintf(" MediaWarp %s ", config.Version().AppVersion), 71, "="))
}

func main() {
	if showVersion {
		versionInfo, _ := json.MarshalIndent(config.Version(), "", "  ")
		fmt.Println(string(versionInfo))
		return
	}

	if isDebug {
		logging.SetLevel(logrus.DebugLevel)
		logging.Info("已启用调试模式")
	}

	signChan := make(chan os.Signal, 1)
	signal.Notify(signChan, syscall.SIGINT, syscall.SIGTERM)
	defer func() {
		logging.Info("MediaWarp 已退出")
	}()

	logging.Info("正在加载配置文件: ", configPath)

	if err := config.Init(configPath); err != nil { // 初始化配置，配置只在启动时读取
		panic("配置初始化失败: " + err.Error())
	}

	logging.Init()                                                                           // 初始化日志
	logging.Infof("上游媒体服务器类型：%s，服务器地址：%s", config.MediaServer.Type, config.MediaServer.ADDR) // 日志打印
	logConfigState()

	if err := handler.Init(); err != nil { // 初始化媒体服务器处理器
		panic("媒体服务器处理器初始化失败: " + err.Error())
	}

	ginR := router.InitRouter() // 路由初始化

	// 先显式监听，端口不可用时立刻给出明确原因，而不是异步失败
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(int(config.Port)))
	if err != nil {
		panic(fmt.Sprintf("端口 %d 无法监听: %v", config.Port, err))
	}

	srv := &http.Server{
		Handler: ginR,
		// 防止 Slowloris 慢速攻击耗尽连接；流媒体是长连接，不设置 ReadTimeout/WriteTimeout
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	logging.Infof("MediaWarp 启动成功，监听端口：%d", config.Port)

	select {
	case sig := <-signChan:
		logging.Info("MediaWarp 正在退出，信号：", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			logging.Warning("HTTP 服务关闭异常：", err)
		}
	case err := <-errCh:
		logging.Error("MediaWarp 运行出错：", err)
	}
}

// 打印启动时的关键配置状态
//
// 配置只在启动时生效，这些提示能帮助定位「改了配置文件却没生效」的问题
func logConfigState() {
	if config.HTTPStrm.Enable && len(config.HTTPStrm.PrefixList) == 0 {
		logging.Warning("http_strm.enable 为 true 但 http_strm.prefix_list 为空，302 重定向不会命中任何 Strm 文件")
	}
	if config.ClientFilter.Enable {
		logging.Infof("客户端过滤已启用，模式：%s，名单：%v", config.ClientFilter.Mode, config.ClientFilter.ClientList)
	}
}
