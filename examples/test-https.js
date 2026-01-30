const https = require('https');

console.log('发送 HTTPS 请求到 httpbin.org...\n');

https.get('https://httpbin.org/headers', (res) => {
  let data = '';
  res.on('data', (chunk) => {
    data += chunk;
  });
  res.on('end', () => {
    console.log('✅ 响应接收成功！\n');
    const response = JSON.parse(data);
    console.log('User-Agent:', response.headers['User-Agent']);
    console.log('\n完整的请求头:');
    console.log(JSON.stringify(response.headers, null, 2));
  });
}).on('error', (err) => {
  console.error('❌ 请求失败:', err.message);
});
