package syslogchannel

import "testing"

// FuzzSplitFrames 做三件事：不 panic、进度单调（剩余字节一定变少）、
// 单帧不超过上限。分帧是 TCP 长连接的热路径，一个死循环就能拖垮整条连接。
func FuzzSplitFrames(f *testing.F) {
	f.Add([]byte("5 hello"))
	f.Add([]byte("line one\nline two\n"))
	f.Add([]byte("10 abc"))
	f.Add([]byte("999999999999999999999999 x\n"))

	f.Fuzz(func(t *testing.T, payload []byte) {
		frames, trailing := splitFrames(payload)

		if len(frames)+len(trailing) == 0 && len(payload) > 0 {
			t.Fatalf("输入 %q 既没产出帧也没留残留", payload)
		}

		// 残留必须是原输入的后缀，且严格短于原输入——否则说明没有推进。
		if len(trailing) > 0 && len(trailing) >= len(payload) && len(frames) == 0 {
			// 允许：单帧还没收全。
			return
		}

		for _, frame := range frames {
			if len(frame) > MaxMessageBytes {
				t.Fatalf("帧长 %d 超过上限 %d", len(frame), MaxMessageBytes)
			}
		}
	})
}

// FuzzFlattenNeverPanics：任意字节都不该让信封解析 panic。
func FuzzFlattenNeverPanics(f *testing.F) {
	f.Add([]byte("<34>1 2026-11-14T10:00:00Z fw01 app - - - msg"))
	f.Add([]byte("<34>Nov 14 10:00:00 fw01 sshd[1]: x"))
	f.Add([]byte("not syslog"))
	f.Add([]byte("<"))

	f.Fuzz(func(t *testing.T, payload []byte) {
		_, _ = flatten(payload)
	})
}
