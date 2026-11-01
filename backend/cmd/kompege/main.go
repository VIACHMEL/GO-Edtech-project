package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"os"
	"strconv"
)

type KompegeTask struct {
    Number     int    `json:"number"`
    Text       string `json:"text"`
    Key        string `json:"key"`
    Difficulty int    `json:"difficulty"`
    SolveText  string `json:"solve_text"`
	SubTasks []SubTask `json:"subTask"`
}
type SubTask struct{
	Number int `json:"number"`
	Text string `json:"text"`
	Key string `json:"key"`
}
func difficultyName(level int) string {
	switch level{
	case 0:
		return "base"
	case 1:
		return "advanced"
	case 2:
		return "high"
	default:
		return "unknown"
	}
}
func getTask(client *http.Client, id int) (KompegeTask, error){
	url:= fmt.Sprintf("https://kompege.ru/api/v1/task/%d", id)

	resp,err:=client.Get(url)

	if err!=nil{
		return KompegeTask{},fmt.Errorf("Запрос задачи %d %w",id,err)
	}
	defer resp.Body.Close()

	if resp.StatusCode!=http.StatusOK{
		return KompegeTask{}, fmt.Errorf("задача %d: HTTP %s", id, resp.Status)
	}

	var kt KompegeTask

	err=json.NewDecoder(resp.Body).Decode(&kt)
	if err!=nil{
		return KompegeTask{}, fmt.Errorf("декодирование задачи %d: %w", id, err)
	}
	return kt,nil
}
func main() {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	if len(os.Args)<2{
		fmt.Println("Укажи ID задания")
		return
	}
	for _,arg:=range os.Args[1:]{
		id,err:=strconv.Atoi(arg)
		if err!=nil{
			fmt.Println("Некоректный ID:",arg)
			continue
		}
		task,err:=getTask(client,id)
		if err!=nil{
			fmt.Print("Ошибка:",err)
			continue
		}
		if task.Number<1||task.Number>27{
			fmt.Println("Пропущен номер:",task.Number)
			continue
		}
		fmt.Printf("ID запроса: %d | № ЕГЭ: %d | Ответ: %s | условие %s\n",id,task.Number,task.Key,task.Text)

	}

	
}
