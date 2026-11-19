package assistant

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chenpinetree/SimCallHub/internal/agent/service"
)

// EncodeWAV wraps 8 kHz 16-bit little-endian mono PCM into a standard RIFF WAVE file.
func EncodeWAV(pcm8k []byte) ([]byte, float64) {
	if len(pcm8k)%2 == 1 {
		pcm8k = pcm8k[:len(pcm8k)-1]
	}
	out := make([]byte, 44+len(pcm8k))
	copy(out, "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(36+len(pcm8k)))
	copy(out[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(out[16:], 16)
	binary.LittleEndian.PutUint16(out[20:], 1) // PCM
	binary.LittleEndian.PutUint16(out[22:], 1) // mono
	binary.LittleEndian.PutUint32(out[24:], 8000)
	binary.LittleEndian.PutUint32(out[28:], 16000)
	binary.LittleEndian.PutUint16(out[32:], 2)
	binary.LittleEndian.PutUint16(out[34:], 16)
	copy(out[36:], "data")
	binary.LittleEndian.PutUint32(out[40:], uint32(len(pcm8k)))
	copy(out[44:], pcm8k)
	dur := math.Round(float64(len(pcm8k))/16000*10) / 10
	return out, dur
}

// DecodeWAV extracts raw PCM bytes from a RIFF WAVE file.
func DecodeWAV(wav []byte) ([]byte, error) {
	if len(wav) < 44 || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return nil, errors.New("无效的 WAV 音频文件")
	}
	if idx := bytes.Index(wav[12:], []byte("data")); idx >= 0 && 12+idx+8 <= len(wav) {
		start := 12 + idx + 8
		size := int(binary.LittleEndian.Uint32(wav[start-4 : start]))
		if start+size <= len(wav) {
			return wav[start : start+size], nil
		}
		return wav[start:], nil
	}
	return wav[44:], nil
}

// Synthesize connects to the configured realtime speech model, synthesizes text
// using cfg.Voice, cfg.SpeechRate, and cfg.Style, and returns an 8 kHz mono WAV.
func Synthesize(ctx context.Context, cfg Config, endpoint, apiKey, text string) ([]byte, float64, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, 0, errors.New("请输入要生成语音的留言文本内容")
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	c, err := dial(ctx, endpoint, apiKey)
	if err != nil {
		return nil, 0, err
	}
	defer c.close()

	events, readErr := c.events(ctx)
	stylePrompt := styles[cfg.Style]
	if stylePrompt == "" {
		stylePrompt = styles[StyleWarm]
	}
	prompt := fmt.Sprintf("你是一个电话语音留言播报员。\n%s\n请只逐字原样、自然地朗读用户提供的语音留言内容，不要添加任何多余的前缀、后缀、解释或寒暄，读完后立即结束：\n「%s」", stylePrompt, text)
	format, err := configure(ctx, c, events, cfg, prompt)
	if err != nil {
		return nil, 0, err
	}

	if cfg.IsDoubao() {
		if err := c.send(ctx, map[string]any{
			"type": "speech_text_buffer.commit",
			"text": text,
		}); err != nil {
			return nil, 0, err
		}
	} else {
		if err := c.send(ctx, map[string]any{
			"type": "conversation.item.create",
			"item": map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "请立即逐字原样朗读这段语音留言：「" + text + "」"},
				},
			},
		}); err != nil {
			return nil, 0, err
		}
		if err := c.send(ctx, map[string]any{"type": "response.create"}); err != nil {
			return nil, 0, err
		}
	}

	down := newDecimator(format.out)
	fast := newStretcher(cfg.SpeechRate)
	var pcm8k []byte

	for {
		select {
		case <-ctx.Done():
			if len(pcm8k) > 0 {
				pcm8k = append(pcm8k, fast.flush()...)
				wav, dur := EncodeWAV(pcm8k)
				return wav, dur, nil
			}
			return nil, 0, errors.New("生成语音超时，请检查网络连接或模型配置")
		case e, ok := <-events:
			if !ok {
				if len(pcm8k) > 0 {
					pcm8k = append(pcm8k, fast.flush()...)
					wav, dur := EncodeWAV(pcm8k)
					return wav, dur, nil
				}
				select {
				case rerr := <-readErr:
					return nil, 0, errors.New("语音接口断开了连接：" + rerr.Error())
				default:
					return nil, 0, errors.New("语音接口断开了连接")
				}
			}
			switch e.Type {
			case "error":
				return nil, 0, errors.New("语音接口报错：" + e.errorText())
			case "response.audio.delta", "response.output_audio.delta":
				if raw, err := base64.StdEncoding.DecodeString(e.Delta); err == nil && len(raw) > 0 {
					speech := down.process(raw)
					pcm8k = append(pcm8k, fast.process(speech)...)
				}
			case "response.audio.done", "response.output_audio.done", "response.done":
				if len(pcm8k) > 0 || e.Type == "response.done" {
					pcm8k = append(pcm8k, fast.flush()...)
					if len(pcm8k) == 0 {
						return nil, 0, errors.New("语音模型没有返回任何音频数据")
					}
					wav, dur := EncodeWAV(pcm8k)
					return wav, dur, nil
				}
			}
		}
	}
}

// SynthesizeVoicemail synthesizes a voicemail WAV using the current model and endpoint settings.
func (a *Agent) SynthesizeVoicemail(ctx context.Context, cfg Config, text string) ([]byte, float64, error) {
	llm, err := a.LLM(ctx)
	if err != nil {
		return nil, 0, err
	}
	endpoint, err := cfg.Endpoint(llm)
	if err != nil {
		return nil, 0, err
	}
	key := cfg.Key(llm)
	if key == "" {
		return nil, 0, errNoKey
	}
	return Synthesize(ctx, cfg, endpoint, key, text)
}

func (a *Agent) voicemailPath(item *VoicemailItem) string {
	if item == nil || item.FileName == "" || a.VoicemailDir == "" {
		return ""
	}
	return filepath.Join(a.VoicemailDir, filepath.Base(item.FileName))
}

func (a *Agent) handleVoicemail(ctx context.Context, cfg Config, call *service.AssistantCall) {
	item := cfg.Voicemail.ActiveItem()
	path := a.voicemailPath(item)
	if item == nil || path == "" {
		call.SetError("未找到启用的语音留言音频")
		call.Note(ctx, "warn", "语音留言接听失败：未找到启用的语音留言音频文件")
		call.Hangup(ctx)
		return
	}
	wav, err := os.ReadFile(path)
	if err != nil {
		call.SetError("读取语音留言文件失败：" + err.Error())
		call.Note(ctx, "warn", "读取语音留言文件失败："+err.Error())
		call.Hangup(ctx)
		return
	}
	pcm, err := DecodeWAV(wav)
	if err != nil {
		call.SetError(err.Error())
		call.Note(ctx, "warn", "解析语音留言 WAV 失败："+err.Error())
		call.Hangup(ctx)
		return
	}

	call.SetPhase(phaseTalking)
	call.Transcript(roleAssistant, fmt.Sprintf("【语音留言 · %s】%s", item.Name, item.Text), true)
	call.Play(pcm)
	call.EndSpeech()

	started := time.Now()
	limit := time.Duration(cmp.Or(cfg.MaxDurationSec, defaultMaxSec)) * time.Second
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-call.Done():
			break loop
		case <-call.Frames():
			// Drain incoming audio frames while the audioBridge records the call.
		case <-tick.C:
			if time.Since(started) >= limit {
				call.SetPhase(phaseEnding)
				_ = call.Hangup(context.WithoutCancel(ctx))
				break loop
			}
		}
	}

	lines := call.Lines()
	summary := Summary{
		Caller:   "来电语音留言",
		Purpose:  fmt.Sprintf("已播放语音留言「%s」，请收听通话录音查看对方留言内容", item.Name),
		Urgency:  "中",
		Callback: "请听通话录音",
		Category: "其他",
		Brief:    fmt.Sprintf("已播放语音留言「%s」并录制通话", item.Name),
		Usage: Usage{
			Format:  "本地 WAV (8000 Hz)",
			TalkSec: math.Round(time.Since(started).Seconds()),
		},
	}
	transcript, _ := json.Marshal(lines)
	note, _ := json.Marshal(summary)
	if err := call.SaveNote(context.WithoutCancel(ctx), string(transcript), string(note)); err != nil {
		a.Log.Warn("save voicemail note", "error", err)
	}
	call.Note(ctx, "info", fmt.Sprintf("语音留言接听结束（%s）：已播放「%s」", call.Number, item.Name))
}
