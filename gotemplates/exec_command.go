package gotemplates

import (
	"fmt"
	"log"
	"os/exec"
)

func execCommand() {
	// bashを使用しないとglobが展開されない
	// 注意: 引数を''をくくるとうまく動かない
	//cmd := exec.Command("bash", "-c", "rm -rfv /home/isucon/private_isu/webapp/public/image/*")

	// 注意: 実行権限を忘れずに
	cmd := exec.Command("bash", "-c", "/home/isucon/remove_grater_than_10000_images.sh")
	fmt.Printf("running command: `%s`\n", cmd.String())
	output, err := cmd.Output()

	if err != nil {
		log.Fatalf("command exec error: %v", err)
	}
	fmt.Printf("command output: %s\n", output)
}
