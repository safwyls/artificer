/* Minimal version.dll proxy: loads UE4SS from the game dir, and answers the
 * real version.dll API with graceful failures (UE treats file-version info
 * as optional metadata). Probe-quality, not production. */
#define WIN32_LEAN_AND_MEAN
#include <windows.h>

BOOL WINAPI DllMain(HINSTANCE inst, DWORD reason, LPVOID reserved) {
    (void)reserved;
    if (reason == DLL_PROCESS_ATTACH) {
        DisableThreadLibraryCalls(inst);
        LoadLibraryA("ue4ss\\UE4SS.dll");
    }
    return TRUE;
}

int __stdcall p_GetFileVersionInfoA(const char*a, unsigned b, unsigned c, void*d){(void)a;(void)b;(void)c;(void)d;SetLastError(1813);return 0;}
int __stdcall p_GetFileVersionInfoW(const wchar_t*a, unsigned b, unsigned c, void*d){(void)a;(void)b;(void)c;(void)d;SetLastError(1813);return 0;}
int __stdcall p_GetFileVersionInfoExA(unsigned e, const char*a, unsigned b, unsigned c, void*d){(void)e;(void)a;(void)b;(void)c;(void)d;SetLastError(1813);return 0;}
int __stdcall p_GetFileVersionInfoExW(unsigned e, const wchar_t*a, unsigned b, unsigned c, void*d){(void)e;(void)a;(void)b;(void)c;(void)d;SetLastError(1813);return 0;}
unsigned __stdcall p_GetFileVersionInfoSizeA(const char*a, unsigned*h){(void)a;if(h)*h=0;SetLastError(1813);return 0;}
unsigned __stdcall p_GetFileVersionInfoSizeW(const wchar_t*a, unsigned*h){(void)a;if(h)*h=0;SetLastError(1813);return 0;}
unsigned __stdcall p_GetFileVersionInfoSizeExA(unsigned e, const char*a, unsigned*h){(void)e;(void)a;if(h)*h=0;SetLastError(1813);return 0;}
unsigned __stdcall p_GetFileVersionInfoSizeExW(unsigned e, const wchar_t*a, unsigned*h){(void)e;(void)a;if(h)*h=0;SetLastError(1813);return 0;}
int __stdcall p_VerQueryValueA(const void*a, const char*b, void**c, unsigned*d){(void)a;(void)b;if(c)*c=0;if(d)*d=0;return 0;}
int __stdcall p_VerQueryValueW(const void*a, const wchar_t*b, void**c, unsigned*d){(void)a;(void)b;if(c)*c=0;if(d)*d=0;return 0;}
unsigned __stdcall p_VerLanguageNameA(unsigned l, char*b, unsigned n){(void)l;(void)b;(void)n;return 0;}
unsigned __stdcall p_VerLanguageNameW(unsigned l, wchar_t*b, unsigned n){(void)l;(void)b;(void)n;return 0;}
unsigned __stdcall p_VerFindFileA(unsigned f, const char*a,const char*b,const char*c,char*d,unsigned*e,char*g,unsigned*h){(void)f;(void)a;(void)b;(void)c;(void)d;(void)e;(void)g;(void)h;return 0;}
unsigned __stdcall p_VerFindFileW(unsigned f, const wchar_t*a,const wchar_t*b,const wchar_t*c,wchar_t*d,unsigned*e,wchar_t*g,unsigned*h){(void)f;(void)a;(void)b;(void)c;(void)d;(void)e;(void)g;(void)h;return 0;}
unsigned __stdcall p_VerInstallFileA(unsigned f, const char*a,const char*b,const char*c,const char*d,const char*e,char*g,unsigned*h){(void)f;(void)a;(void)b;(void)c;(void)d;(void)e;(void)g;(void)h;return 0x00000001;}
unsigned __stdcall p_VerInstallFileW(unsigned f, const wchar_t*a,const wchar_t*b,const wchar_t*c,const wchar_t*d,const wchar_t*e,wchar_t*g,unsigned*h){(void)f;(void)a;(void)b;(void)c;(void)d;(void)e;(void)g;(void)h;return 0x00000001;}
