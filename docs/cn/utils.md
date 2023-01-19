# utils.go

`utils.go` 放一些没有明确分类的小工具


- [utils.go](#utilsgo)
	- [JSON](#json)
	- [IsHasField](#ishasfield)
	- [IsHasMethod](#ishasmethod)
	- [ValidateFileHash](#validatefilehash)
	- [GetFuncName](#getfuncname)
	- [FallBack](#fallback)
	- [RegexNamedSubMatch](#regexnamedsubmatch)
	- [FlattenMap](#flattenmap)
	- [ForceGCUnBlocking](#forcegcunblocking)
	- [AutoGC](#autogc)
	- [TemplateWithMap](#templatewithmap)
	- [URLMasking](#urlmasking)
	- [SetStructFieldsBySlice](#setstructfieldsbyslice)
	- [UniqueStrings](#uniquestrings)
	- [RemoveEmpty](#removeempty)
	- [TrimEleSpaceAndRemoveEmpty](#trimelespaceandremoveempty)
	- [InArray](#inarray)
	- [IsPtr](#isptr)
	- [RunCMD](#runcmd)
	- [Base64Encode](#base64encode)
	- [ExpCache](#expcache)
	- [ExpiredMap](#expiredmap)


## JSON

`github.com/json-iterator/go` 的封装，提供常用的 JSON 序列化/反序列化。

```go
gutils.JSON.Marshal
gutils.JSON.UnMarshal


gutils.JSON.MarshalToString
gutils.JSON.UnmarshalFromString
```

## IsHasField

```go
func IsHasField(st any, fieldName string) bool
```

判断 struct 中是否包含某个 field


## IsHasMethod

```go
IsHasMethod(st any, methodName string) bool
```

判断 struct 中是否包含某个 method


## ValidateFileHash

```go
ValidateFileHash(filepath string, hashed string) error
```

读取文件内容并比较哈希。

`hashed` 的格式形如 `sha256:xxx`，目前只支持了 SHA256/MD5。


## GetFuncName

```go
GetFuncName(f any) string
```

获取函数名


## FallBack

```go
FallBack(orig func() any, fallback any) (ret any)
```

有时候需要调用一些可能会 panic 的函数，但是我们可以容忍这个函数调用失败。
所以可以设置一个默认值，当调用失败的时候，就返回这个默认值。

1. 首先调用 orig()
2. 调用成功则返回 orig() 的结果
3. 调用失败（panic）就返回 fallback


## RegexNamedSubMatch

```go
RegexNamedSubMatch(r *regexp.Regexp, str string, subMatchMap map[string]string) error
```

正则匹配，可以把正则中的 named group 以 map 的形式返回。


```go
func ExampleRegexNamedSubMatch() {
	reg := regexp.MustCompile(`(?P<key>\d+.*)`)
	str := "12345abcde"
	groups := map[string]string{}
	if err := RegexNamedSubMatch(reg, str, groups); err != nil {
		Logger.Error("try to group match got error", zap.Error(err))
	}

    fmt.Printf("got: %+v", groups)
    // Output: map[string]string{"key": 12345}
}
```

## FlattenMap

```go
FlattenMap(data map[string]any, delimiter string)
```

把嵌套 map 展平，将父 key + delimiter + 子 key 作为新的 key 名。

```go
func ExampleFlattenMap() {
	data := map[string]any{
		"a": "1",
		"b": map[string]any{
			"c": 2,
			"d": map[string]any{
				"e": 3,
			},
		},
    }

	FlattenMap(data, ".")
	// Output: {"a": "1", "b__c": 2, "b__d__e": 3}
}
```

## ForceGCUnBlocking

```go
ForceGCUnBlocking()
```

启动 GC，并且释放内存缓冲区。



## AutoGC

```go
AutoGC(ctx context.Context, opts ...GcOptFunc) (err error)
```

监控内存使用量，当内存到达 85% 时启动强制 GC。

内存比例可以通过 `gutils.WithGCMemRatio` 设置。

内存 limit 文件路径可以通过 `gutils.WithGCMemLimitFilePath` 设置，默认为 `"/sys/fs/cgroup/memory/memory.limit_in_bytes"`。


## TemplateWithMap

```go
TemplateWithMap(tpl string, data map[string]any) string
```

将 tpl 中的 `"${key}"` 替换为 data 中该 key 所对应的值。


## URLMasking

```go
URLMasking(url, mask string) string
```

简单地去掉 URL 重的账户密码。

## SetStructFieldsBySlice

```go
SetStructFieldsBySlice(structs, vals any) (err error)
```

用 slices 给 struct 赋值。

## UniqueStrings

```go
UniqueStrings(vs []string) (r []string)
```

返回去重后的 slice


## RemoveEmpty

```go
RemoveEmpty(vs []string) (r []string)
```

去除 slice 中的空元素

## TrimEleSpaceAndRemoveEmpty

```go
TrimEleSpaceAndRemoveEmpty(vs []string) (r []string)
```

对 slice 中的每一个元素做 `TrimSpace` 去除首位空格，然后再去除所有空元素。

## InArray

```go
InArray(collection any, ele any) bool
```

判断元素是否在 array/slice 中

## IsPtr

```go
IsPtr(t any) bool
```

是否是指针

## RunCMD

```go
RunCMD(ctx context.Context, app string, args ...string) (stdout []byte, err error)
```

执行 shell 命令

## Base64Encode

```go
Base64Encode(raw []byte) string
Base64Decode(encoded string) ([]byte, error)
```

base64 的序列化/反序列化。

## ExpCache

```go
NewExpCache(ctx context.Context, ttl time.Duration) *ExpCache
```

ExpCache 是一个带过期时间的 map，有两个方法：

```go
func (c *ExpCache) Store(key, val any)
func (c *ExpCache) Load(key any) (data any, ok bool)
```

存储（store）的时候，会自动为该 key 计算一个 expiration = now() + ttl。
到达这个时间后就会自动删除。

## ExpiredMap

```go
NewExpiredMap(ctx context.Context, ttl time.Duration, new func() any) (el *ExpiredMap, err error)
```

类似于 ExpCache，也是一个带 ttl 的 map。

不过只允许 Get，不允许 Store，每一次 Get 都会自动刷新过期时间。

当 Get 一个不存在的 key 时，会调用初始化时传入的 `new()` 方法生成一个新对象并返回。

```go
func (e *ExpiredMap) Get(key string) any
```
