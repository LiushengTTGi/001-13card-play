// Command kuaile13 starts 开心十三张: a single binary that serves the embedded
// table UI on localhost and opens it in the default browser.
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/kuaile13/kuaile13/internal/server"
)

//go:embed all:web
var webAssets embed.FS

// version is overwritten by -ldflags "-X main.version=..." in the release build.
var version = "dev"

func main() {
	port := flag.Int("port", 8362, "监听端口，0 表示自动选择空闲端口")
	host := flag.String("host", "127.0.0.1", "监听地址，改成 0.0.0.0 可以让局域网访问")
	bet := flag.Float64("bet", 1, "每家下注倍数")
	think := flag.Duration("think", 700*time.Millisecond, "电脑每家出牌的间隔")
	noBrowser := flag.Bool("no-browser", false, "只启动服务，不自动打开浏览器")
	showVersion := flag.Bool("version", false, "打印版本号")
	flag.Parse()

	if *showVersion {
		fmt.Printf("开心十三张 kuaile13 %s (%s %s/%s)\n", version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		return
	}

	web, err := fs.Sub(webAssets, "web")
	if err != nil {
		fmt.Fprintln(os.Stderr, "kuaile13: 内嵌网页资源缺失:", err)
		os.Exit(1)
	}

	ln, err := listen(*host, *port)
	if err != nil {
		fmt.Fprintln(os.Stderr, "kuaile13:", err)
		os.Exit(1)
	}

	table := server.NewTable(*bet, *think)
	stopClock := table.Run()
	defer stopClock()

	addr := fmt.Sprintf("http://%s/", ln.Addr().String())
	fmt.Printf("开心十三张 %s 已启动: %s\n", version, addr)
	fmt.Println("关闭此窗口即可退出游戏。")

	srv := &http.Server{
		Handler:           server.Handlers{Table: table, Web: web}.Mux(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	if !*noBrowser {
		time.Sleep(150 * time.Millisecond)
		if err := openBrowser(addr); err != nil {
			fmt.Println("请手动在浏览器打开:", addr)
		}
	}

	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "kuaile13: 服务异常:", err)
		os.Exit(1)
	}
}

// listen prefers the requested port and falls back to any free one.
func listen(host string, port int) (net.Listener, error) {
	if port != 0 {
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
		if err == nil {
			return ln, nil
		}
	}
	return net.Listen("tcp", host+":0")
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
