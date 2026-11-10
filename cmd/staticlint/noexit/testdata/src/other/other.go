package other

import "os"

// Функция с именем main в пакете, который не называется main, — не предмет
// проверки: os.Exit здесь допустим.
func main() {
	os.Exit(1)
}
