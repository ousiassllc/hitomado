//go:build android && cgo

package androidbg

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>

static JavaVM* hs_vm = NULL;

// 呼び出しごとに、Worker が渡した SecretBridge のグローバル参照を作る（複数の呼び出しが重ならないよう、グローバル変数に持たない）。
static jobject hs_newGlobal(JNIEnv* env, jobject obj) {
    if (hs_vm == NULL && (*env)->GetJavaVM(env, &hs_vm) != 0) {
        return NULL;
    }
    return (*env)->NewGlobalRef(env, obj);
}

static void hs_deleteGlobal(JNIEnv* env, jobject obj) {
    if (obj != NULL) {
        (*env)->DeleteGlobalRef(env, obj);
    }
}

static const char* hs_chars(JNIEnv* env, jstring s) {
    if (s == NULL) return NULL;
    return (*env)->GetStringUTFChars(env, s, NULL);
}

static void hs_release(JNIEnv* env, jstring s, const char* c) {
    if (s != NULL && c != NULL) (*env)->ReleaseStringUTFChars(env, s, c);
}

static jstring hs_newString(JNIEnv* env, const char* c) {
    return (*env)->NewStringUTF(env, c);
}

// target.<method>(String) -> String を呼び、結果を malloc した C 文字列で返す（呼び出し側が free）。失敗は NULL。
// Go のゴルーチンは任意の OS スレッドで動くため、JNIEnv が無ければアタッチし、終わったらデタッチする。
static char* hs_call(jobject target, const char* method, const char* arg) {
    if (hs_vm == NULL || target == NULL) return NULL;
    JNIEnv* env = NULL;
    int detach = 0;
    jint r = (*hs_vm)->GetEnv(hs_vm, (void**)&env, JNI_VERSION_1_6);
    if (r == JNI_EDETACHED) {
        if ((*hs_vm)->AttachCurrentThread(hs_vm, &env, NULL) != 0) return NULL;
        detach = 1;
    } else if (r != JNI_OK) {
        return NULL;
    }
    char* out = NULL;
    jclass cls = (*env)->GetObjectClass(env, target);
    if (cls != NULL) {
        jmethodID mid = (*env)->GetMethodID(env, cls, method, "(Ljava/lang/String;)Ljava/lang/String;");
        if (mid != NULL) {
            jstring jarg = (*env)->NewStringUTF(env, arg);
            jstring jres = (jstring)(*env)->CallObjectMethod(env, target, mid, jarg);
            if ((*env)->ExceptionCheck(env)) {
                (*env)->ExceptionClear(env);
            } else if (jres != NULL) {
                const char* chars = (*env)->GetStringUTFChars(env, jres, NULL);
                if (chars != NULL) {
                    out = strdup(chars);
                    (*env)->ReleaseStringUTFChars(env, jres, chars);
                }
            }
            if (jres != NULL) (*env)->DeleteLocalRef(env, jres);
            if (jarg != NULL) (*env)->DeleteLocalRef(env, jarg);
        } else if ((*env)->ExceptionCheck(env)) {
            (*env)->ExceptionClear(env);
        }
        (*env)->DeleteLocalRef(env, cls);
    }
    if (detach) (*hs_vm)->DetachCurrentThread(hs_vm);
    return out;
}
*/
import "C"

import (
	"unsafe"

	"github.com/ousiassllc/hitomado/internal/headless"
	"github.com/ousiassllc/hitomado/internal/secrets"
)

// Java_com_wails_app_HeadlessSync_nativeSyncOnce は全アカウントを 1 回同期し、結果の JSON（headless.Encode）を返す。
// dataDir は Worker が getFilesDir() から組み立てたデータフォルダ、secrets は SecretBridge（資格情報の読み書き）。
//
//export Java_com_wails_app_HeadlessSync_nativeSyncOnce
func Java_com_wails_app_HeadlessSync_nativeSyncOnce(env *C.JNIEnv, _ C.jclass, jdir C.jstring, jver C.jstring, target C.jobject) C.jstring {
	dir := goString(env, jdir)
	version := goString(env, jver)
	ref := C.hs_newGlobal(env, target)
	defer C.hs_deleteGlobal(env, ref)

	store := secrets.NewBridgeStore(secrets.NewEnvelopeBridge(func(method, arg string) (string, bool) {
		return callTarget(ref, method, arg)
	}))
	out := headless.Run(headless.Options{DataDir: dir, Secrets: store, Version: version})

	cout := C.CString(out)
	defer C.free(unsafe.Pointer(cout))
	return C.hs_newString(env, cout)
}

// Java_com_wails_app_HeadlessSync_nativeCancel は実行中のヘッドレス同期を取り消す（WorkManager が Worker を止めたとき）。
//
//export Java_com_wails_app_HeadlessSync_nativeCancel
func Java_com_wails_app_HeadlessSync_nativeCancel(_ *C.JNIEnv, _ C.jclass) {
	headless.Cancel()
}

func goString(env *C.JNIEnv, s C.jstring) string {
	c := C.hs_chars(env, s)
	if c == nil {
		return ""
	}
	defer C.hs_release(env, s, c)
	return C.GoString(c)
}

func callTarget(ref C.jobject, method, arg string) (string, bool) {
	cm := C.CString(method)
	ca := C.CString(arg)
	defer C.free(unsafe.Pointer(cm))
	defer C.free(unsafe.Pointer(ca))
	res := C.hs_call(ref, cm, ca)
	if res == nil {
		return "", false
	}
	defer C.free(unsafe.Pointer(res))
	return C.GoString(res), true
}
