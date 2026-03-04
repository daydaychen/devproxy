package proxy

import (
	"fmt"
	"net/http"
)

// RequestPlugin 定义了一类可以在请求发送到上游前修改 *http.Request 的插件
type RequestPlugin interface {
	// Name 返回插件的名字，用于配置匹配
	Name() string
	// ProcessRequest 拦截并修改请求体。返回 error 则中断代理。
	ProcessRequest(req *http.Request) error
}

// pluginRegistry 插件注册表
var pluginRegistry = map[string]RequestPlugin{}

func init() {
	// 注册内置插件
	RegisterPlugin(&CodexFixPlugin{})
}

// RegisterPlugin 注册一个请求插件
func RegisterPlugin(plugin RequestPlugin) {
	pluginRegistry[plugin.Name()] = plugin
}

// GetPlugin 根据名称获取插件实例
func GetPlugin(name string) (RequestPlugin, error) {
	p, ok := pluginRegistry[name]
	if !ok {
		return nil, fmt.Errorf("插件 %s 未找到", name)
	}
	return p, nil
}
