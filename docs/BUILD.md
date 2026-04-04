# 跨平台编译指南

本文档介绍如何使用 `build.py` 脚本编译跨平台动态库。

## 环境要求

### 通用要求
- Go 1.21+
- Python 3.8+

### 平台特定要求

| 平台 | 额外要求 |
|------|---------|
| Windows | MinGW-w64 (用于 CGO 编译) |
| Linux | GCC |
| macOS | Xcode Command Line Tools |
| iOS | Xcode + iOS SDK |

### 安装 MinGW-w64 (Windows)

```powershell
# 使用 Chocolatey
choco install mingw

# 或使用 Scoop
scoop install mingw
```

## 快速开始

### 交互式模式

直接运行脚本，进入交互式菜单：

```bash
python build.py
```

输出：
```
==================================================
Chimera Remote Port Forward 编译脚本
==================================================

当前系统: windows (amd64)

请选择编译目标:
  1. 当前系统 (自动检测)
  2. Windows DLL (amd64)
  3. Windows DLL (arm64)
  4. Linux .so (amd64)
  5. Linux .so (arm64)
  6. macOS .dylib (amd64)
  7. macOS .dylib (arm64)
  8. iOS 静态库 (arm64)
  9. iOS 模拟器静态库
  10. 编译全部 (交叉编译)
  0. 退出

请输入选项 (0-10):
```

### 命令行模式

指定目标平台进行编译：

```bash
# 编译 Windows DLL
python build.py -t windows

# 编译 Linux .so
python build.py -t linux

# 编译 macOS .dylib
python build.py -t macos

# 编译 iOS 静态库
python build.py -t ios

# 编译全部平台
python build.py -t all

# 编译多个目标
python build.py -t windows linux macos
```

## 命令行参数

| 参数 | 说明 | 示例 |
|------|------|------|
| `-t, --target` | 编译目标平台 | `-t windows` |
| `-o, --output` | 输出目录 (默认: `./build`) | `-o ./dist` |
| `-a, --arch` | 目标架构 (默认: `amd64`) | `-a arm64` |
| `-v, --version` | 显示版本信息 | `-v` |

## 目标平台选项

| 目标 | 输出文件 | 说明 |
|------|---------|------|
| `windows` | `chimera-port.dll` | Windows AMD64 |
| `windows-arm64` | `chimera-port_arm64.dll` | Windows ARM64 |
| `linux` | `libchimera-port.so` | Linux AMD64 |
| `linux-arm64` | `libchimera-port_arm64.so` | Linux ARM64 |
| `macos` | `libchimera-port.dylib` | macOS AMD64 (Intel) |
| `macos-arm64` | `libchimera-port_arm64.dylib` | macOS ARM64 (Apple Silicon) |
| `ios` | `libchimera-port_ios.a` | iOS 设备 (ARM64) |
| `ios-sim` | `libchimera-port_sim_*.a` | iOS 模拟器 |
| `all` | 全部平台 | 交叉编译所有目标 |
| `native` | 自动检测 | 根据当前系统编译 |

## 输出文件结构

编译完成后，输出文件位于 `./build/` 目录：

```
build/
├── chimera-port.dll              # Windows AMD64
├── chimera-port_arm64.dll        # Windows ARM64
├── chimera-port.h                # Windows 头文件
├── libchimera-port.so            # Linux AMD64
├── libchimera-port_arm64.so      # Linux ARM64
├── libchimera-port.dylib         # macOS AMD64
├── libchimera-port_arm64.dylib   # macOS ARM64
├── libchimera-port_ios.a         # iOS 设备
├── libchimera-port_ios.h         # iOS 头文件
├── libchimera-port_sim_arm64.a   # iOS 模拟器 ARM64
└── libchimera-port_sim_amd64.a   # iOS 模拟器 AMD64
```

## 使用示例

### 示例 1: 编译当前平台

```bash
python build.py -t native
```

自动检测当前系统并编译：
- Windows → `chimera-port.dll`
- Linux → `libchimera-port.so`
- macOS → `libchimera-port.dylib`

### 示例 2: 编译 Windows 和 Linux

```bash
python build.py -t windows linux
```

### 示例 3: 指定输出目录

```bash
python build.py -t windows -o ./dist/libs
```

### 示例 4: 编译全部平台

```bash
python build.py -t all
```

## Flutter FFI 集成

### 1. 添加依赖

在 `pubspec.yaml` 中添加：

```yaml
dependencies:
  ffi: ^2.1.0
```

### 2. 加载动态库

```dart
import 'dart:ffi';
import 'dart:io';

final DynamicLibrary lib;

if (Platform.isWindows) {
  lib = DynamicLibrary.open('chimera-port.dll');
} else if (Platform.isLinux) {
  lib = DynamicLibrary.open('libchimera-port.so');
} else if (Platform.isMacOS) {
  lib = DynamicLibrary.open('libchimera-port.dylib');
} else if (Platform.isIOS) {
  lib = DynamicLibrary.process();
}
```

### 3. 绑定函数

```dart
// Initialize
typedef InitializeNative = Int32 Function(Pointer<Utf8> server, Pointer<Utf8> token);
typedef InitializeDart = int Function(Pointer<Utf8> server, Pointer<Utf8> token);

final initialize = lib.lookupFunction<InitializeNative, InitializeDart>('Initialize');

// AddPort
typedef AddPortNative = Int32 Function(Pointer<Utf8> deviceName, Int32 localPort);
typedef AddPortDart = int Function(Pointer<Utf8> deviceName, int localPort);

final addPort = lib.lookupFunction<AddPortNative, AddPortDart>('AddPort');
```

## 常见问题

### Q: Windows 编译失败，提示找不到 gcc

确保已安装 MinGW-w64 并添加到 PATH：

```powershell
gcc --version
```

### Q: CGO 编译失败

确保设置 `CGO_ENABLED=1`，脚本已自动处理。

### Q: iOS 编译失败

iOS 编译需要在 macOS 上进行，并需要安装 Xcode。

### Q: 交叉编译提示找不到编译器

交叉编译需要对应平台的交叉编译工具链。例如：
- Windows 交叉编译 Linux: 需要 `x86_64-linux-gnu-gcc`
- Linux 交叉编译 Windows: 需要 `x86_64-w64-mingw32-gcc`

## API 参考

编译后的库提供以下导出函数：

| 函数 | 说明 |
|------|------|
| `Initialize(server, token)` | 初始化连接 |
| `SetStateCallback(cb)` | 设置状态回调 |
| `AddPort(deviceName, localPort)` | 添加端口转发 |
| `RemovePort(deviceName)` | 移除端口转发 |
| `GetPortCount()` | 获取端口数量 |
| `GetPortInfo(index, ...)` | 获取端口信息 |
| `Stop()` | 停止所有连接 |
| `GetVersion()` | 获取版本号 |

详细 API 文档请参考 [API 文档](./docs/api.md)。
