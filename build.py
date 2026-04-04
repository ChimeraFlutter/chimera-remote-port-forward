#!/usr/bin/env python3
"""
Cross-platform build script for chimera-remote-port-forward
Builds DLL (Windows), .so (Linux), .dylib (macOS), .a (iOS)
"""

import argparse
import os
import platform
import subprocess
import sys
from pathlib import Path

# 项目配置
PROJECT_NAME = "chimera-port"
DLL_ENTRY = "./cmd/dll"
OUTPUT_DIR = "./build"

# 版本信息
VERSION = "1.0.0"


def get_system_info() -> tuple[str, str]:
    """获取当前系统信息"""
    system = platform.system().lower()
    machine = platform.machine().lower()

    # 统一架构名称
    if machine in ("x86_64", "amd64"):
        machine = "amd64"
    elif machine in ("arm64", "aarch64"):
        machine = "arm64"
    elif machine in ("x86", "i386", "i686"):
        machine = "386"

    return system, machine


def check_go() -> bool:
    """检查Go环境"""
    try:
        result = subprocess.run(
            ["go", "version"],
            capture_output=True,
            text=True
        )
        if result.returncode == 0:
            print(f"Go环境: {result.stdout.strip()}")
            return True
    except FileNotFoundError:
        pass

    print("错误: 未找到Go环境，请先安装Go")
    return False


def check_mingw() -> bool:
    """检查MinGW环境 (Windows需要)"""
    if platform.system().lower() != "windows":
        return True

    # 检查 gcc
    try:
        result = subprocess.run(
            ["gcc", "--version"],
            capture_output=True,
            text=True
        )
        if result.returncode == 0:
            print(f"MinGW GCC: {result.stdout.split()[0] if result.stdout else 'OK'}")
            return True
    except FileNotFoundError:
        pass

    print("警告: Windows编译需要MinGW-w64")
    print("请安装: choco install mingw 或从 https://www.mingw-w64.org/ 下载")
    return False


def build_windows_dll(output_dir: Path, arch: str = "amd64") -> bool:
    """编译Windows DLL"""
    print(f"\n{'='*50}")
    print(f"编译 Windows DLL ({arch})")
    print(f"{'='*50}")

    # 输出文件名: chimera-port.dll 或 chimera-port_arm64.dll
    if arch == "amd64":
        output_file = output_dir / f"{PROJECT_NAME}.dll"
    else:
        output_file = output_dir / f"{PROJECT_NAME}_{arch}.dll"

    # 设置环境变量
    env = os.environ.copy()
    env["GOOS"] = "windows"
    env["GOARCH"] = arch
    env["CGO_ENABLED"] = "1"

    # 设置CC编译器
    if arch == "amd64":
        env["CC"] = "x86_64-w64-mingw32-gcc"
    elif arch == "arm64":
        env["CC"] = "aarch64-w64-mingw32-gcc"
    else:
        env["CC"] = "i686-w64-mingw32-gcc"

    # 构建命令
    cmd = [
        "go", "build",
        "-buildmode=c-shared",
        "-ldflags", f"-s -w -X main.Version={VERSION}",
        "-trimpath",
        "-o", str(output_file),
        DLL_ENTRY
    ]

    print(f"输出文件: {output_file}")

    try:
        result = subprocess.run(cmd, env=env, check=True)
        if output_file.exists():
            print(f"成功: {output_file}")
            return True
    except subprocess.CalledProcessError as e:
        print(f"编译失败: {e}")
        return False

    return False


def build_linux_so(output_dir: Path, arch: str = "amd64") -> bool:
    """编译Linux .so 动态库"""
    print(f"\n{'='*50}")
    print(f"编译 Linux .so ({arch})")
    print(f"{'='*50}")

    # 输出文件名: libchimera-port.so 或 libchimera-port_arm64.so
    if arch == "amd64":
        output_file = output_dir / f"lib{PROJECT_NAME}.so"
    else:
        output_file = output_dir / f"lib{PROJECT_NAME}_{arch}.so"

    env = os.environ.copy()
    env["GOOS"] = "linux"
    env["GOARCH"] = arch
    env["CGO_ENABLED"] = "1"

    cmd = [
        "go", "build",
        "-buildmode=c-shared",
        "-ldflags", f"-s -w -X main.Version={VERSION}",
        "-trimpath",
        "-o", str(output_file),
        DLL_ENTRY
    ]

    print(f"输出文件: {output_file}")

    try:
        result = subprocess.run(cmd, env=env, check=True)
        if output_file.exists():
            print(f"成功: {output_file}")
            return True
    except subprocess.CalledProcessError as e:
        print(f"编译失败: {e}")
        return False

    return False


def build_macos_lib(output_dir: Path, arch: str = "amd64") -> bool:
    """编译macOS动态库 (.dylib)"""
    print(f"\n{'='*50}")
    print(f"编译 macOS .dylib ({arch})")
    print(f"{'='*50}")

    env = os.environ.copy()
    env["GOOS"] = "darwin"
    env["GOARCH"] = arch
    env["CGO_ENABLED"] = "1"

    # 输出文件名: libchimera-port.dylib 或 libchimera-port_arm64.dylib
    if arch == "amd64":
        dylib_file = output_dir / f"lib{PROJECT_NAME}.dylib"
    else:
        dylib_file = output_dir / f"lib{PROJECT_NAME}_{arch}.dylib"

    print(f"\n编译动态库: {dylib_file}")

    cmd_dylib = [
        "go", "build",
        "-buildmode=c-shared",
        "-ldflags", f"-s -w -X main.Version={VERSION}",
        "-trimpath",
        "-o", str(dylib_file),
        DLL_ENTRY
    ]

    try:
        subprocess.run(cmd_dylib, env=env, check=True)
        if dylib_file.exists():
            print(f"成功: {dylib_file}")
            return True
        else:
            return False
    except subprocess.CalledProcessError as e:
        print(f"动态库编译失败: {e}")
        return False


def build_ios_static(output_dir: Path, arch: str = "arm64", simulator: bool = False) -> bool:
    """编译iOS静态库 (.a)"""
    print(f"\n{'='*50}")
    print(f"编译 iOS .a ({arch}){' - 模拟器' if simulator else ''}")
    print(f"{'='*50}")

    env = os.environ.copy()
    env["GOOS"] = "ios"
    env["GOARCH"] = arch
    env["CGO_ENABLED"] = "1"

    # 输出文件名
    if simulator:
        if arch == "arm64":
            output_file = output_dir / f"lib{PROJECT_NAME}_sim_arm64.a"
        else:
            output_file = output_dir / f"lib{PROJECT_NAME}_sim_amd64.a"
    else:
        output_file = output_dir / f"lib{PROJECT_NAME}_ios.a"

    print(f"\n编译静态库: {output_file}")

    cmd = [
        "go", "build",
        "-buildmode=c-archive",
        "-ldflags", f"-s -w -X main.Version={VERSION}",
        "-trimpath",
        "-o", str(output_file),
        DLL_ENTRY
    ]

    try:
        subprocess.run(cmd, env=env, check=True)
        if output_file.exists():
            print(f"成功: {output_file}")
            return True
        else:
            return False
    except subprocess.CalledProcessError as e:
        print(f"静态库编译失败: {e}")
        return False


def build_cross_platform(targets: list[str], output_dir: Path) -> None:
    """交叉编译多个平台"""
    results = {}

    for target in targets:
        target = target.lower().strip()

        if target in ("windows", "win", "dll"):
            results["windows_amd64"] = build_windows_dll(output_dir, "amd64")

        elif target in ("windows-arm64", "win-arm64", "dll-arm64"):
            results["windows_arm64"] = build_windows_dll(output_dir, "arm64")

        elif target in ("windows-386", "win-386", "dll-386"):
            results["windows_386"] = build_windows_dll(output_dir, "386")

        elif target in ("linux", "so"):
            results["linux_amd64"] = build_linux_so(output_dir, "amd64")

        elif target in ("linux-arm64", "so-arm64"):
            results["linux_arm64"] = build_linux_so(output_dir, "arm64")

        elif target in ("macos", "darwin", "dylib"):
            results["macos_amd64"] = build_macos_lib(output_dir, "amd64")

        elif target in ("macos-arm64", "darwin-arm64"):
            results["macos_arm64"] = build_macos_lib(output_dir, "arm64")

        elif target in ("ios", "iphone"):
            results["ios_arm64"] = build_ios_static(output_dir, "arm64", simulator=False)

        elif target in ("ios-sim", "iphone-sim"):
            results["ios_sim_arm64"] = build_ios_static(output_dir, "arm64", simulator=True)
            results["ios_sim_amd64"] = build_ios_static(output_dir, "amd64", simulator=True)

        elif target == "all":
            # 编译所有目标
            results["windows_amd64"] = build_windows_dll(output_dir, "amd64")
            results["windows_arm64"] = build_windows_dll(output_dir, "arm64")
            results["linux_amd64"] = build_linux_so(output_dir, "amd64")
            results["linux_arm64"] = build_linux_so(output_dir, "arm64")
            results["macos_amd64"] = build_macos_lib(output_dir, "amd64")
            results["macos_arm64"] = build_macos_lib(output_dir, "arm64")
            results["ios_arm64"] = build_ios_static(output_dir, "arm64", simulator=False)
            results["ios_sim_arm64"] = build_ios_static(output_dir, "arm64", simulator=True)
            results["ios_sim_amd64"] = build_ios_static(output_dir, "amd64", simulator=True)

        else:
            print(f"未知目标: {target}")

    # 打印结果摘要
    print(f"\n{'='*50}")
    print("编译结果摘要")
    print(f"{'='*50}")

    for target, success in results.items():
        status = "成功" if success else "失败"
        print(f"  {target}: {status}")


def build_native(output_dir: Path) -> None:
    """根据当前系统自动编译"""
    system, arch = get_system_info()

    print(f"检测到系统: {system} ({arch})")

    if system == "windows":
        build_windows_dll(output_dir, arch)
    elif system == "linux":
        build_linux_so(output_dir, arch)
    elif system == "darwin":
        build_macos_lib(output_dir, arch)
    else:
        print(f"不支持的系统: {system}")


def interactive_mode() -> None:
    """交互式模式"""
    print("\n" + "="*50)
    print("Chimera Remote Port Forward 编译脚本")
    print("="*50)

    system, arch = get_system_info()
    print(f"\n当前系统: {system} ({arch})")

    print("\n请选择编译目标:")
    print("  1. 当前系统 (自动检测)")
    print("  2. Windows DLL (amd64)")
    print("  3. Windows DLL (arm64)")
    print("  4. Linux .so (amd64)")
    print("  5. Linux .so (arm64)")
    print("  6. macOS .dylib (amd64)")
    print("  7. macOS .dylib (arm64)")
    print("  8. iOS 静态库 (arm64)")
    print("  9. iOS 模拟器静态库")
    print("  10. 编译全部 (交叉编译)")
    print("  0. 退出")

    while True:
        try:
            choice = input("\n请输入选项 (0-10): ").strip()

            if choice == "0":
                print("退出")
                sys.exit(0)

            output_dir = Path(OUTPUT_DIR)
            output_dir.mkdir(exist_ok=True)

            if choice == "1":
                build_native(output_dir)
            elif choice == "2":
                build_windows_dll(output_dir, "amd64")
            elif choice == "3":
                build_windows_dll(output_dir, "arm64")
            elif choice == "4":
                build_linux_so(output_dir, "amd64")
            elif choice == "5":
                build_linux_so(output_dir, "arm64")
            elif choice == "6":
                build_macos_lib(output_dir, "amd64")
            elif choice == "7":
                build_macos_lib(output_dir, "arm64")
            elif choice == "8":
                build_ios_static(output_dir, "arm64", simulator=False)
            elif choice == "9":
                build_ios_static(output_dir, "arm64", simulator=True)
                build_ios_static(output_dir, "amd64", simulator=True)
            elif choice == "10":
                build_cross_platform(["all"], output_dir)
            else:
                print("无效选项，请重新输入")
                continue

            break

        except KeyboardInterrupt:
            print("\n已取消")
            sys.exit(0)


def main():
    parser = argparse.ArgumentParser(
        description="Chimera Remote Port Forward 跨平台编译脚本",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
示例:
  %(prog)s                      # 交互式模式
  %(prog)s -t windows           # 编译Windows DLL (amd64)
  %(prog)s -t windows-arm64     # 编译Windows DLL (arm64)
  %(prog)s -t linux             # 编译Linux .so (amd64)
  %(prog)s -t macos             # 编译macOS .dylib (amd64)
  %(prog)s -t ios               # 编译iOS静态库
  %(prog)s -t ios-sim           # 编译iOS模拟器静态库
  %(prog)s -t all               # 编译全部平台
  %(prog)s -t windows linux     # 编译多个目标

输出文件:
  Windows AMD64  -> chimera-port.dll
  Windows ARM64  -> chimera-port_arm64.dll
  Linux AMD64    -> libchimera-port.so
  Linux ARM64    -> libchimera-port_arm64.so
  macOS AMD64    -> libchimera-port.dylib
  macOS ARM64    -> libchimera-port_arm64.dylib
  iOS 设备       -> libchimera-port_ios.a
  iOS 模拟器     -> libchimera-port_sim_arm64.a, libchimera-port_sim_amd64.a
        """
    )

    parser.add_argument(
        "-t", "--target",
        nargs="+",
        choices=["windows", "windows-arm64", "linux", "linux-arm64",
                 "macos", "macos-arm64", "ios", "ios-sim", "all", "native"],
        help="编译目标平台"
    )

    parser.add_argument(
        "-o", "--output",
        default=OUTPUT_DIR,
        help=f"输出目录 (默认: {OUTPUT_DIR})"
    )

    parser.add_argument(
        "-a", "--arch",
        default="amd64",
        choices=["amd64", "386", "arm64"],
        help="目标架构 (默认: amd64)"
    )

    parser.add_argument(
        "-v", "--version",
        action="version",
        version=f"%(prog)s {VERSION}"
    )

    args = parser.parse_args()

    # 检查Go环境
    if not check_go():
        sys.exit(1)

    output_dir = Path(args.output)
    output_dir.mkdir(exist_ok=True)

    # 根据参数选择模式
    if args.target:
        if "native" in args.target:
            build_native(output_dir)
        else:
            build_cross_platform(args.target, output_dir)
    else:
        # 无参数时进入交互式模式
        interactive_mode()


if __name__ == "__main__":
    main()
