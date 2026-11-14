package syslogchannel

import (
	"bytes"
	"strconv"
)

// MaxMessageBytes 是单条 syslog 报文上限，与 spec 一致；超限截断。
const MaxMessageBytes = 64 << 10

// splitFrames 把一段 TCP 流拆成完整报文。
//
// RFC6587 有两种分帧：八位组计数（"123 <报文>"）与非透明分帧（换行分隔）。
// 两种都必须在实网出现——同一台设备的不同版本可能各用一种，
// 因此按首字符判断而不是配置。
// trailing 是尚未完整的剩余字节，调用方留到下一次读取继续拼接。
func splitFrames(buffer []byte) (frames [][]byte, trailing []byte) {
	for len(buffer) > 0 {
		frame, rest, complete := nextFrame(buffer)
		if !complete {
			return frames, buffer
		}

		frames = append(frames, frame)
		buffer = rest
	}

	return frames, nil
}

// nextFrame 取出一帧；complete 为 false 表示数据还不够。
func nextFrame(buffer []byte) (frame, rest []byte, complete bool) {
	// 八位组计数：以十进制数字开头，后跟空格，再跟指定长度的报文。
	if digitEnd := leadingDigits(buffer); digitEnd > 0 && digitEnd < len(buffer) && buffer[digitEnd] == ' ' {
		length, err := strconv.Atoi(string(buffer[:digitEnd]))
		if err != nil || length < 0 {
			// 数字非法：退回非透明分帧，避免整条流卡死。
			return nextLineFrame(buffer)
		}

		start := digitEnd + 1
		if len(buffer) < start+length {
			return nil, buffer, false
		}

		return buffer[start : start+length], buffer[start+length:], true
	}

	return nextLineFrame(buffer)
}

// nextLineFrame 按换行取一帧；没有换行说明这条报文还没收完。
func nextLineFrame(buffer []byte) (frame, rest []byte, complete bool) {
	if index := bytes.IndexByte(buffer, '\n'); index >= 0 {
		return bytes.TrimRight(buffer[:index], "\r"), buffer[index+1:], true
	}

	return nil, buffer, false
}

// leadingDigits 返回开头连续数字的字节数。
func leadingDigits(buffer []byte) int {
	index := 0
	for index < len(buffer) && buffer[index] >= '0' && buffer[index] <= '9' {
		index++
	}

	return index
}

// truncate 按上限截断报文，返回是否发生了截断。
func truncate(frame []byte) ([]byte, bool) {
	if len(frame) <= MaxMessageBytes {
		return frame, false
	}

	return frame[:MaxMessageBytes], true
}
