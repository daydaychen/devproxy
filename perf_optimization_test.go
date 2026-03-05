package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

// ============================================================================
// 1. 字符串操作优化：strings.Builder vs 拼接
// ============================================================================

// 方式 1: 使用 + 拼接字符串 (最差性能)
func concatString(n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += "hello"
	}
	return result
}

// 方式 2: 使用 fmt.Sprintf (较差性能)
func sprintfString(n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result = fmt.Sprintf("%s%s", result, "hello")
	}
	return result
}

// 方式 3: 使用 strings.Builder (推荐，最佳性能)
func stringBuilder(n int) string {
	var builder strings.Builder
	for i := 0; i < n; i++ {
		builder.WriteString("hello")
	}
	return builder.String()
}

// 方式 4: 使用 strings.Builder 预分配容量 (最优性能)
func stringBuilderPreAlloc(n int) string {
	var builder strings.Builder
	builder.Grow(n * 5) // "hello" = 5 bytes
	for i := 0; i < n; i++ {
		builder.WriteString("hello")
	}
	return builder.String()
}

// 方式 5: 使用 bytes.Buffer (适用于二进制数据)
func bytesBufferString(n int) string {
	var buffer bytes.Buffer
	for i := 0; i < n; i++ {
		buffer.WriteString("hello")
	}
	return buffer.String()
}

// 方式 6: 使用字节切片预分配 (适用于已知大小的场景)
func byteSlicePreAlloc(n int) string {
	result := make([]byte, 0, n*5)
	for i := 0; i < n; i++ {
		result = append(result, []byte("hello")...)
	}
	return string(result)
}

func BenchmarkStringConcat(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = concatString(100)
	}
}

func BenchmarkStringSprintf(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = sprintfString(100)
	}
}

func BenchmarkStringBuilder(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = stringBuilder(100)
	}
}

func BenchmarkStringBuilderPreAlloc(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = stringBuilderPreAlloc(100)
	}
}

func BenchmarkBytesBuffer(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = bytesBufferString(100)
	}
}

func BenchmarkByteSlicePreAlloc(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = byteSlicePreAlloc(100)
	}
}

// ============================================================================
// 2. Map 预分配和容量设置
// ============================================================================

// 方式 1: 无预分配 (最差性能)
func mapNoPreAlloc(n int) map[int]int {
	m := make(map[int]int)
	for i := 0; i < n; i++ {
		m[i] = i * 2
	}
	return m
}

// 方式 2: 精确预分配 (最佳性能)
func mapExactPreAlloc(n int) map[int]int {
	m := make(map[int]int, n)
	for i := 0; i < n; i++ {
		m[i] = i * 2
	}
	return m
}

// 方式 3: 过度预分配 (浪费内存)
func mapOverPreAlloc(n int) map[int]int {
	m := make(map[int]int, n*10)
	for i := 0; i < n; i++ {
		m[i] = i * 2
	}
	return m
}

// 方式 4: 预分配 50% 容量 (折中方案)
func mapHalfPreAlloc(n int) map[int]int {
	m := make(map[int]int, n/2)
	for i := 0; i < n; i++ {
		m[i] = i * 2
	}
	return m
}

// 方式 5: 使用 map 字面量 (无法预分配)
func mapLiteral(n int) map[int]int {
	m := map[int]int{}
	for i := 0; i < n; i++ {
		m[i] = i * 2
	}
	return m
}

func BenchmarkMapNoPreAlloc(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = mapNoPreAlloc(1000)
	}
}

func BenchmarkMapExactPreAlloc(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = mapExactPreAlloc(1000)
	}
}

func BenchmarkMapOverPreAlloc(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = mapOverPreAlloc(1000)
	}
}

func BenchmarkMapHalfPreAlloc(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = mapHalfPreAlloc(1000)
	}
}

func BenchmarkMapLiteral(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = mapLiteral(1000)
	}
}

// ============================================================================
// 3. 避免内存逃逸的模式
// ============================================================================

// 场景 1: 返回局部变量指针 (会导致逃逸)
func escapeToHeap(n int) *int {
	result := n * 2
	return &result // 逃逸到堆
}

// 场景 2: 返回值而非指针 (避免逃逸)
func noEscape(n int) int {
	result := n * 2
	return result // 保留在栈上
}

// 场景 3: 接口转换导致逃逸
func interfaceEscape(n int) interface{} {
	result := n * 2
	return &result // 逃逸到堆
}

// 场景 4: 闭包捕获变量导致逃逸
func closureEscape(n int) func() int {
	return func() int {
		return n * 2 // n 逃逸到堆
	}
}

// 场景 5: 大结构体值传递 (可能逃逸)
type LargeStruct struct {
	data [1024]byte
}

func largeStructEscape(n int) LargeStruct {
	return LargeStruct{data: [1024]byte{byte(n)}}
}

func largeStructPointer(n int) *LargeStruct {
	return &LargeStruct{data: [1024]byte{byte(n)}}
}

// 场景 6: 切片操作避免逃逸
func sliceEscape(n int) []int {
	data := make([]int, n)
	for i := range data {
		data[i] = i
	}
	return data // 返回切片本身不会导致底层数组逃逸（如果只在函数内使用）
}

// 场景 7: 使用 sync.Pool 复用对象
var largeStructPool = &sync.Pool{
	New: func() interface{} {
		return &LargeStruct{}
	},
}

func pooledLargeStruct(n int) *LargeStruct {
	s := largeStructPool.Get().(*LargeStruct)
	s.data[0] = byte(n)
	return s
}

func BenchmarkEscapeToHeap(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = escapeToHeap(42)
	}
}

func BenchmarkNoEscape(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = noEscape(42)
	}
}

func BenchmarkInterfaceEscape(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = interfaceEscape(42)
	}
}

func BenchmarkClosureEscape(b *testing.B) {
	for i := 0; i < b.N; i++ {
		fn := closureEscape(42)
		_ = fn()
	}
}

func BenchmarkLargeStructValue(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = largeStructEscape(42)
	}
}

func BenchmarkLargeStructPointer(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = largeStructPointer(42)
	}
}

// ============================================================================
// 4. 零拷贝技术（bytes.Buffer, io.Reader 复用）
// ============================================================================

// 场景 1: 重复创建 bytes.Buffer (低效)
func createBufferEachTime(data []byte) []byte {
	var result []byte
	for i := 0; i < 10; i++ {
		buf := &bytes.Buffer{}
		buf.Write(data)
		result = buf.Bytes()
	}
	return result
}

// 场景 2: 复用 bytes.Buffer (高效)
func reuseBuffer(data []byte, buf *bytes.Buffer) []byte {
	buf.Reset()
	for i := 0; i < 10; i++ {
		buf.Write(data)
	}
	return buf.Bytes()
}

// 场景 3: 使用 bytes.Buffer 的 Grow 预分配
func bufferWithGrow(data []byte) []byte {
	buf := &bytes.Buffer{}
	buf.Grow(len(data) * 10)
	for i := 0; i < 10; i++ {
		buf.Write(data)
	}
	return buf.Bytes()
}

// 场景 4: 使用 bytes.Reader 实现零拷贝读取
func zeroCopyRead(data []byte) ([]byte, error) {
	reader := bytes.NewReader(data)
	var result []byte
	buf := make([]byte, 1024)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// 场景 5: 使用 io.CopyBuffer 零拷贝
func ioCopyBuffer(src []byte) ([]byte, error) {
	reader := bytes.NewReader(src)
	writer := &bytes.Buffer{}
	writer.Grow(len(src))
	buf := make([]byte, 4096)
	_, err := io.CopyBuffer(writer, reader, buf)
	return writer.Bytes(), err
}

// 场景 6: 直接使用切片（真正的零拷贝）
func directSlice(data []byte) []byte {
	return data
}

// 场景 7: strings.Builder 用于字符串构建（零拷贝）
func stringBuilderZeroCopy(parts []string) string {
	var builder strings.Builder
	totalLen := 0
	for _, p := range parts {
		totalLen += len(p)
	}
	builder.Grow(totalLen)
	for _, p := range parts {
		builder.WriteString(p)
	}
	return builder.String()
}

// 场景 8: 避免不必要的字符串转换
func avoidStringConvert(data []byte) string {
	// 错误做法：多次转换
	// s := string(data)
	// return strings.ToUpper(s)

	// 正确做法：直接在字节层面操作
	result := make([]byte, len(data))
	for i, b := range data {
		if b >= 'a' && b <= 'z' {
			result[i] = b - 32
		} else {
			result[i] = b
		}
	}
	return string(result)
}

var globalBuffer bytes.Buffer

func BenchmarkCreateBufferEachTime(b *testing.B) {
	data := []byte("hello world")
	for i := 0; i < b.N; i++ {
		_ = createBufferEachTime(data)
	}
}

func BenchmarkReuseBuffer(b *testing.B) {
	data := []byte("hello world")
	buf := &bytes.Buffer{}
	for i := 0; i < b.N; i++ {
		_ = reuseBuffer(data, buf)
	}
}

func BenchmarkBufferWithGrow(b *testing.B) {
	data := []byte("hello world")
	for i := 0; i < b.N; i++ {
		_ = bufferWithGrow(data)
	}
}

func BenchmarkZeroCopyRead(b *testing.B) {
	data := make([]byte, 1024)
	for i := 0; i < b.N; i++ {
		_, _ = zeroCopyRead(data)
	}
}

func BenchmarkIoCopyBuffer(b *testing.B) {
	data := make([]byte, 1024)
	for i := 0; i < b.N; i++ {
		_, _ = ioCopyBuffer(data)
	}
}

func BenchmarkDirectSlice(b *testing.B) {
	data := make([]byte, 1024)
	for i := 0; i < b.N; i++ {
		_ = directSlice(data)
	}
}

func BenchmarkStringBuilderZeroCopy(b *testing.B) {
	parts := []string{"hello", " ", "world", "!", " ", "Go", " ", "performance"}
	for i := 0; i < b.N; i++ {
		_ = stringBuilderZeroCopy(parts)
	}
}

func BenchmarkAvoidStringConvert(b *testing.B) {
	data := []byte("hello world")
	for i := 0; i < b.N; i++ {
		_ = avoidStringConvert(data)
	}
}

// ============================================================================
// 5. Go Compiler 优化相关最佳实践 (2024-2026)
// ============================================================================

// 优化 1: 使用内联提示 (Go 1.20+)
//go:noinline
func noInlineFunc(x int) int {
	return x * 2
}

//go:inline
func inlineFunc(x int) int {
	return x * 2
}

// 优化 2: 使用 bounds check elimination
func boundsCheckElimination(data []int, index int) int {
	// 编译器可以消除第二次访问的边界检查
	if index >= 0 && index < len(data) {
		_ = data[index]      // 有边界检查
		return data[index]   // 边界检查被消除
	}
	return 0
}

// 优化 3: 使用 range 而非索引遍历
func rangeIteration(data []int) int {
	sum := 0
	for _, v := range data {
		sum += v
	}
	return sum
}

func indexIteration(data []int) int {
	sum := 0
	for i := 0; i < len(data); i++ {
		sum += data[i]
	}
	return sum
}

// 优化 4: 使用 append 预分配容量
func appendPreAlloc(n int) []int {
	result := make([]int, 0, n)
	for i := 0; i < n; i++ {
		result = append(result, i)
	}
	return result
}

func appendNoPreAlloc(n int) []int {
	var result []int
	for i := 0; i < n; i++ {
		result = append(result, i)
	}
	return result
}

// 优化 5: 使用 copy 而非循环
func copyWithLoop(src, dst []int) {
	for i := 0; i < len(src); i++ {
		dst[i] = src[i]
	}
}

func copyWithBuiltin(src, dst []int) {
	copy(dst, src)
}

// 优化 6: 避免在循环中分配内存
func allocInLoop(n int) int {
	sum := 0
	for i := 0; i < n; i++ {
		s := fmt.Sprintf("%d", i) // 每次循环分配
		sum += len(s)
	}
	return sum
}

func allocOutsideLoop(n int) int {
	sum := 0
	var buf [32]byte // 栈上分配
	for i := 0; i < n; i++ {
		s := string(buf[:])
		sum += len(s)
	}
	return sum
}

// 优化 7: 使用位运算代替算术运算
func arithmeticOps(x int) int {
	return x * 8 / 4
}

func bitwiseOps(x int) int {
	return (x << 3) >> 2
}

// 优化 8: 使用 switch 代替多个 if-else
func multipleIfElse(x int) string {
	if x == 1 {
		return "one"
	} else if x == 2 {
		return "two"
	} else if x == 3 {
		return "three"
	} else if x == 4 {
		return "four"
	} else {
		return "other"
	}
}

func switchStatement(x int) string {
	switch x {
	case 1:
		return "one"
	case 2:
		return "two"
	case 3:
		return "three"
	case 4:
		return "four"
	default:
		return "other"
	}
}

func BenchmarkNoInlineFunc(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = noInlineFunc(42)
	}
}

func BenchmarkInlineFunc(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = inlineFunc(42)
	}
}

func BenchmarkBoundsCheckElimination(b *testing.B) {
	data := make([]int, 1000)
	for i := 0; i < b.N; i++ {
		_ = boundsCheckElimination(data, 500)
	}
}

func BenchmarkRangeIteration(b *testing.B) {
	data := make([]int, 1000)
	for i := 0; i < b.N; i++ {
		_ = rangeIteration(data)
	}
}

func BenchmarkIndexIteration(b *testing.B) {
	data := make([]int, 1000)
	for i := 0; i < b.N; i++ {
		_ = indexIteration(data)
	}
}

func BenchmarkAppendPreAlloc(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = appendPreAlloc(1000)
	}
}

func BenchmarkAppendNoPreAlloc(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = appendNoPreAlloc(1000)
	}
}

func BenchmarkCopyWithLoop(b *testing.B) {
	src := make([]int, 1000)
	dst := make([]int, 1000)
	for i := 0; i < b.N; i++ {
		copyWithLoop(src, dst)
	}
}

func BenchmarkCopyWithBuiltin(b *testing.B) {
	src := make([]int, 1000)
	dst := make([]int, 1000)
	for i := 0; i < b.N; i++ {
		copyWithBuiltin(src, dst)
	}
}

func BenchmarkAllocInLoop(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = allocInLoop(1000)
	}
}

func BenchmarkAllocOutsideLoop(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = allocOutsideLoop(1000)
	}
}

func BenchmarkArithmeticOps(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = arithmeticOps(42)
	}
}

func BenchmarkBitwiseOps(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = bitwiseOps(42)
	}
}

func BenchmarkMultipleIfElse(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = multipleIfElse(3)
	}
}

func BenchmarkSwitchStatement(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = switchStatement(3)
	}
}

func main() {
	fmt.Println("Go 性能优化基准测试")
	fmt.Println("=================")
	fmt.Println("运行命令：go test -bench=. -benchmem perf_optimization_test.go")
}
