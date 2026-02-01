# Smart-Proxy 测试示例

本目录包含用于测试 devproxy 的示例脚本。

## 示例 0: 配置文件模式 (推荐)

使用配置文件可以避免冗长的命令行参数。

### devproxy.yaml (成组规则)

```yaml
rules:
  - name: "httpbin-headers"
    match: ["httpbin.org/headers"]
    overwrite:
      User-Agent: "HeaderBot"
  - name: "httpbin-get"
    match: ["httpbin.org/get"]
    overwrite:
      User-Agent: "GetBot"
      X-Custom: "Value"

verbose: true
```

**运行命令**:
```bash
devproxy -- node examples/test-multiple.js
```

## 示例 1: 简单的 HTTP 请求测试

### test-http.js

```javascript
const http = require('http');

console.log('发送 HTTP 请求到 httpbin.org...');

http.get('http://httpbin.org/headers', (res) => {
  let data = '';
  res.on('data', (chunk) => {
    data += chunk;
  });
  res.on('end', () => {
    console.log('响应数据:');
    console.log(JSON.parse(data));
  });
}).on('error', (err) => {
  console.error('请求失败:', err);
});
```

**运行命令**:
```bash
devproxy --match "httpbin.org" --overwrite useragent=TestBot --verbose -- node examples/test-http.js
```

## 示例 2: HTTPS 请求测试

### test-https.js

```javascript
const https = require('https');

console.log('发送 HTTPS 请求到 httpbin.org...');

https.get('https://httpbin.org/headers', (res) => {
  let data = '';
  res.on('data', (chunk) => {
    data += chunk;
  });
  res.on('end', () => {
    console.log('响应数据:');
    const response = JSON.parse(data);
    console.log('User-Agent:', response.headers['User-Agent']);
    console.log('所有请求头:', response.headers);
  });
}).on('error', (err) => {
  console.error('请求失败:', err);
});
```

**运行命令**:
```bash
devproxy --match "httpbin.org" --overwrite useragent=SecureBot --verbose -- node examples/test-https.js
```

## 示例 3: 多个请求测试

### test-multiple.js

```javascript
const https = require('https');

function makeRequest(url, name) {
  console.log(`\n[${name}] 请求: ${url}`);
  
  https.get(url, (res) => {
    let data = '';
    res.on('data', (chunk) => {
      data += chunk;
    });
    res.on('end', () => {
      console.log(`[${name}] 响应状态码: ${res.statusCode}`);
      console.log(`[${name}] 响应大小: ${data.length} 字节`);
    });
  }).on('error', (err) => {
    console.error(`[${name}] 请求失败:`, err.message);
  });
}

// 测试多个不同的请求
makeRequest('https://httpbin.org/get', 'Test 1');
makeRequest('https://httpbin.org/user-agent', 'Test 2');
makeRequest('https://httpbin.org/headers', 'Test 3');

console.log('\n所有请求已发送，等待响应...');
```

**运行命令**:
```bash
devproxy --match "httpbin.org" --overwrite useragent=MultiBot --verbose -- node examples/test-multiple.js
```

## 示例 4: 带上游代理的测试

假设你本地有 Clash 运行在 7890 端口：

```bash
devproxy \
    --upstream http://127.0.0.1:7890 \
    --match "google.com" \
    --overwrite useragent=ProxyBot \
    --verbose \
    -- curl -v https://www.google.com
```

## 验证 User-Agent 是否被修改

使用以下命令验证 UA 修改是否生效：

```bash
# 正常请求（不通过代理）
curl http://httpbin.org/headers

# 通过 devproxy（应该看到修改后的 User-Agent）
devproxy --match "httpbin.org" --overwrite useragent=MyCustomBot -- curl http://httpbin.org/headers
```

对比两次输出，第二次的 `User-Agent` 应该是 `MyCustomBot`。
