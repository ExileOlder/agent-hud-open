// Portable Windows console entry point. The HUD executable owns all quota
// and authorization code; this file only forwards the console and argv.
#define WIN32_LEAN_AND_MEAN
#define _WIN32_WINNT 0x0601
#include <windows.h>

enum { max_chars = 32768 };
static WCHAR image_path[max_chars];
static WCHAR command_line[max_chars];
static STARTUPINFOEXW startup;
static PROCESS_INFORMATION child;

// Keep a fixed UTF-16 executable name and preserve the caller's original
// argument tail byte-for-byte, including quoting and empty arguments.
static BOOL prepare_command_line(void) {
    static const WCHAR leaf[] = L"BalanceHUD.exe";
    static const WCHAR prefix[] = L"\"BalanceHUD.exe\" --antigravity-helper";
    DWORD length = GetModuleFileNameW(NULL, image_path, max_chars);
    if (!length || length >= max_chars) return FALSE;
    DWORD slash = length;
    while (slash && image_path[slash - 1] != L'\\' && image_path[slash - 1] != L'/') --slash;
    if (!slash || slash + ARRAYSIZE(leaf) > max_chars) return FALSE;
    for (DWORD i = 0; i < ARRAYSIZE(leaf); ++i) image_path[slash + i] = leaf[i];

    const WCHAR *tail = GetCommandLineW();
    BOOL quoted = FALSE;
    while (*tail) {
        if (*tail == L'"') quoted = !quoted;
        else if (!quoted && (*tail == L' ' || *tail == L'\t')) break;
        ++tail;
    }
    DWORD at = 0;
    for (; at + 1 < ARRAYSIZE(prefix); ++at) command_line[at] = prefix[at];
    while (*tail && at + 1 < max_chars) command_line[at++] = *tail++;
    if (*tail) return FALSE;
    command_line[at] = L'\0';
    return TRUE;
}

static HANDLE child_stdio(DWORD id, DWORD access) {
    HANDLE parent = GetStdHandle(id);
    HANDLE process = GetCurrentProcess(), inherited = NULL;
    if (parent && parent != INVALID_HANDLE_VALUE) {
        if (DuplicateHandle(process, parent, process, &inherited, 0, TRUE, DUPLICATE_SAME_ACCESS))
            return inherited;
        return NULL;
    }
    SECURITY_ATTRIBUTES mode = { sizeof(SECURITY_ATTRIBUTES), NULL, TRUE };
    inherited = CreateFileW(L"NUL", access, FILE_SHARE_READ | FILE_SHARE_WRITE,
        &mode, OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, NULL);
    return inherited == INVALID_HANDLE_VALUE ? NULL : inherited;
}

static BOOL launch_child(void) {
    HANDLE io[] = {
        child_stdio(STD_INPUT_HANDLE, GENERIC_READ),
        child_stdio(STD_OUTPUT_HANDLE, GENERIC_WRITE),
        child_stdio(STD_ERROR_HANDLE, GENERIC_WRITE)
    };
    if (!io[0] || !io[1] || !io[2]) return FALSE;
    SIZE_T attrs_size = 0;
    InitializeProcThreadAttributeList(NULL, 1, 0, &attrs_size);
    LPPROC_THREAD_ATTRIBUTE_LIST attrs = HeapAlloc(GetProcessHeap(), 0, attrs_size);
    if (!attrs || !InitializeProcThreadAttributeList(attrs, 1, 0, &attrs_size)) return FALSE;

    startup.StartupInfo.cb = (DWORD)sizeof(startup);
    startup.StartupInfo.dwFlags = STARTF_USESTDHANDLES;
    startup.StartupInfo.hStdInput = io[0];
    startup.StartupInfo.hStdOutput = io[1];
    startup.StartupInfo.hStdError = io[2];
    startup.lpAttributeList = attrs;
    BOOL ok = UpdateProcThreadAttribute(attrs, 0, PROC_THREAD_ATTRIBUTE_HANDLE_LIST,
        io, sizeof(io), NULL, NULL) &&
        CreateProcessW(image_path, command_line, NULL, NULL, TRUE,
            EXTENDED_STARTUPINFO_PRESENT, NULL, NULL, &startup.StartupInfo, &child);
    DeleteProcThreadAttributeList(attrs);
    HeapFree(GetProcessHeap(), 0, attrs);
    for (DWORD i = 0; i < ARRAYSIZE(io); ++i) CloseHandle(io[i]);
    return ok;
}

void balance_monitor_entry(void) {
    DWORD status = 1;
    if (prepare_command_line() && launch_child()) {
        CloseHandle(child.hThread);
        if (WaitForSingleObject(child.hProcess, INFINITE) == WAIT_OBJECT_0)
            GetExitCodeProcess(child.hProcess, &status);
        CloseHandle(child.hProcess);
    } else {
        static const char error[] = "agy-auth-monitor: could not start BalanceHUD.exe\r\n";
        DWORD sent;
        WriteFile(GetStdHandle(STD_ERROR_HANDLE), error, (DWORD)(sizeof(error) - 1), &sent, NULL);
    }
    ExitProcess(status);
}
