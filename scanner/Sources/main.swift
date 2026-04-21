import Darwin
import Foundation

if let s = ProcessInfo.processInfo.environment["MACWIFI_PARENT_PID"],
   let pid = pid_t(s), pid > 0 {
    watchParentPID(pid)
}

guard let portStr = ProcessInfo.processInfo.environment["MACWIFI_PORT"],
      let port = UInt16(portStr) else {
    FileHandle.standardError.write("MACWIFI_PORT not set\n".data(using: .utf8)!)
    exit(1)
}

runService(port: port)
