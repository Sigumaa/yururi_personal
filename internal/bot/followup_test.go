package bot

import (
	"testing"
)

func TestLooksLikePromiseOnly(t *testing.T) {
	if !looksLikePromiseOnly("必要そうなところから順に手をつけますね") {
		t.Fatal("expected promise-like reply to be detected")
	}
	if !looksLikePromiseOnly("承知しました。いま確認しますね。") {
		t.Fatal("expected filler plus promise reply to be detected")
	}
	if looksLikePromiseOnly("監視を登録しておきましたよ") {
		t.Fatal("expected completed factual reply to pass")
	}
	if looksLikePromiseOnly("もちろん、どっちも今ここで答えるね。つまり、今すぐ終わる作業はその場で実行して、継続監視だけ job を更新します。") {
		t.Fatal("expected explanatory reply to pass")
	}
	if looksLikePromiseOnly("了解です。\n- create\n- move") {
		t.Fatal("expected multi-line progress list to pass")
	}
}
