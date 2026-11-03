# modernc.org/libc 上流への報告文（下書き）

> 関連: [Android ビルド](android.md) §12

`modernc.org/libc` の android/amd64 の問題を、上流（<https://gitlab.com/cznic/libc>）へ報告するための下書き。**まだ送っていない**（外部サービスへの投稿・アカウント登録は、この Issue（#83）の範囲外。送るかどうかは人が決める）。送るときは、既存の issue [cznic/libc#41](https://gitlab.com/cznic/libc/-/issues/41)（2024-08 に報告され、2025-04-10 に「No feedback, closing.」で閉じられた。修正はされていない）に、下の本文を新しいコメントとして足す（または新規 issue にして #41 を参照する）。上流の CONTRIBUTING.md の指示に従うこと。

調べた日: 2026-11-03。`modernc.org/libc` v1.79.0（`go.mod` の版）・v1.79.1・v1.80.0（最新タグ。2026-11-01）で、`ccgo_linux_amd64.go` の旧式 syscall の呼び出し箇所（`X__syscall*` から `SYS_open`／`SYS_stat`／`SYS_lstat`／`SYS_access`／`SYS_mkdir`／`SYS_unlink`／`SYS_rename`／`SYS_rmdir` を呼ぶ 15 か所）が同じで、`syscall_musl.go` は v1.79.0 と v1.80.0 でバイト単位で同一。つまり**上流では直っていない**。

---

## 本文（英語）

**Title:** linux/amd64 on Android: legacy syscalls (open/stat/lstat/…) are killed by seccomp (SIGSYS) — working patch

Refs #41 (closed for lack of feedback; the problem is still present in v1.80.0).

**Symptom.** An app using `modernc.org/sqlite` (and so `modernc.org/libc`) built for `GOOS=android GOARCH=amd64` (an x86_64 emulator or device) dies on the first file operation inside the app process:

```
Fatal signal 31 (SIGSYS), code 1 (SYS_SECCOMP), syscall 6
```

**Cause.** Android's app seccomp filter is generated from bionic's `SYSCALLS.TXT` plus the allow/block lists
(<https://android.googlesource.com/platform/bionic/+/refs/heads/main/libc/SYSCALLS.TXT>). On x86_64 it lists the `*at` forms, and of the old numbers only `open`, `stat` and `readlink` are allowed (bionic's `SECCOMP_ALLOWLIST_COMMON.TXT`). The other legacy numbers that exist only on x86_64 (`lstat`, `access`, `mkdir`, `rmdir`, `unlink`, `rename`, `link`, `symlink`, `chmod`, `chown`, `lchown`, `mknod`, `pipe`, `dup2`, …) are not allowed, so the process is killed. The musl C code translated by ccgo issues them directly, for example `_fstatat_kstat` uses `SYS_lstat`, and `open()` is `___syscall_cp(SYS_open, …)`. `linux/arm64` is not affected: the arm64 kernel ABI has no legacy numbers, and `ccgo_linux_arm64.go` does not even define `SYS_open`, `SYS_stat`, … (the amd64 file defines all of them). The Go runtime avoids the problem the same way ([golang/go#27797](https://github.com/golang/go/issues/27797)).

**Patch (proof of concept).** There is no `android` build constraint on `syscall_musl.go`, but `GOOS=android` does satisfy `linux`, so the fix can be a file with `//go:build android && amd64` (the pure-Go `unix` package is already a dependency). `___syscall_cp` and `X__syscall1/2/3` first try `androidLegacySyscall(n, a1, a2, a3)`, which maps the legacy numbers (including the three that happen to be allowed, so that the result does not depend on the Android version) to `openat`/`newfstatat`/`faccessat`/`mkdirat`/`unlinkat`/`renameat2`/`linkat`/`symlinkat`/`readlinkat`/`fchmodat`/`fchownat`/`mknodat`/`pipe2`/`dup3`, and returns `ok=false` for everything else. On every other target a stub returns `ok=false`, so nothing changes there.

The change (`legacy_android_amd64.go` and `legacy_other.go` added, `syscall_musl.go` edited so the four entry points call the hook first) is in our repository as `build/android/libc/amd64.patch` (applies to v1.79.0 with `patch -p1`):
<https://github.com/ousiassllc/hitomado/blob/main/build/android/libc/amd64.patch> *(replace with the real link when the repository is public)*

**Verification.** Android 15 (API 35) x86_64 emulator, an app embedding `modernc.org/sqlite` v1.61.1: with the patch, the app opens its database (creating the file, running migrations) and keeps running; without the patch it dies with the SIGSYS above. Checked against bionic's lists: after the patch, the amd64 file operations that SQLite uses are all in the allowed set. Calls not covered because SQLite never uses them: `poll`, `select`, `epoll_*`, `eventfd`, `inotify_init`, `fork`, `utimes`, `pause`, `signalfd`.

**Suggested direction for a real fix.** ccgo could emit the `*at` forms for these calls on linux/amd64 (as it effectively does on arm64 and other architectures that lack the legacy numbers), or the hook could live in `libc_musl_linux_amd64.go` as suggested in #41. Happy to adapt the patch to the form you prefer.
