package main

import (
	"ProjectSpace/util"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

//用表示执行远程命令结果的结构体
type ResultInfo struct {
	host string
	msg  string
}

//命令执行器，绑定主机信息并执行命令
type executor struct {
	host string
	user string
	pwd  string
	cmd  []string
	file string
}

//输出文件的接口
type OutFile interface {
	io.Closer
	io.Reader
	io.Writer
	Sync() error
}

//执行远程命令并将结果输出到结果文件中，这里可以传入共享内存文件
func (e *executor) execute(outFile OutFile, filepath string, filename string) {
	fmt.Println(e)
	ChanConcurrency <- struct{}{}

	var msg string
	var resFile *MFile
	var code int
	var err error
	var done chan struct{}
	var mvDir string

	success := true
	C := newSSH(e.host, "22", e.user, e.pwd, 0)
	if C == nil {
		success = false
		code = -1
		goto finish
	}
	done = make(chan struct{})
	go keepAlive(C.sshClient, done)
	if e.file != "" {
		code = C.execShellFile(e.file, outFile)
		if code != 0 {
			success = false
		}
	} else {
		for _, cmd := range e.cmd {
			code = C.execCommand(cmd, outFile)
			if code != 0 {
				success = false
				break
			}
		}
	}
finish:
	if success {
		msg = "success"
		mvDir = successDir
		ChanSuccess <- ResultInfo{e.host, msg}
		resFile = FILE_SUCCESS
	} else {
		msg = "ExitCode: " + strconv.Itoa(code)
		mvDir = failedDir
		ChanFailed <- ResultInfo{e.host, msg}
		resFile = FILE_FAILED
	}
	defer func() {
		if C != nil {
			err = C.sshClient.Close()
			err = C.sftpClient.Close()
			if done != nil {
				done <- struct{}{}
			}
		}
	}()
	_ = outFile.Sync()
	_ = outFile.Close()
	fmt.Printf("ip: %s\tcode: %d\n", e.host, code)
	newPath := fmt.Sprintf("%s/%s", mvDir, filename)
	mv(filepath, newPath)
	resFile.writeAll(e.host + "\t" + msg + "\n")
	_ = resFile.Sync()
	<-ChanConcurrency
	ChanFinished <- struct{}{}
}

//用于给文件增加readAll和writeAll方法，试用更方便
type MFile struct {
	*os.File
}

//读取文件所有内容
func (f *MFile) readAll() []byte {
	content := bytes.Buffer{}
	buffer := make([]byte, 1024)
	n := 0
	var err error
	for {
		n, err = f.Read(buffer)
		if err != nil {
			break
		}
		content.Write(buffer[:n])
	}
	return content.Bytes()
}

//向文件中写入所有数据
func (f *MFile) writeAll(data interface{}) {
	var tmp []byte
	switch value := data.(type) {
	case string:
		tmp = []byte(value)
	case []byte:
		tmp = value
	}
	length := len(tmp)
	count := 0
	var err error
	var n int
	for count < length {
		n, err = f.Write(tmp[count:])
		if err != nil {
			break
		}
		count += n
	}
	_ = f.Sync()
}

//遍历一个路径下所有文件，并通过handle函数处理文件
func walk(path string, handle func(string, os.FileInfo)) {
	state, err := os.Stat(path)
	if err != nil {
		return
	}
	var infoList []os.FileInfo
	if !state.IsDir() {
		infoList = append(infoList, state)
	} else {
		infoList, err = ioutil.ReadDir(path)
		handleError(err, "ReadDir")
	}
	for i := 0; i < len(infoList); i++ {
		info := infoList[i]
		curPath := filepath.Join(path, info.Name())
		if info.IsDir() {
			walk(curPath, handle)
		} else {
			handle(curPath, info)
		}
	}
}

//处理复制失败
func ErrorCopy(err error) {
	if err != nil {
		handleError(err, "复制失败")
		return
	}
}

//复制文件的函数
func copyFile(src, dst string) {
	srcFile, err := os.Open(src)
	ErrorCopy(err)
	dstFile, _ := os.Create(dst)
	ErrorCopy(err)
	_, err = io.Copy(dstFile, srcFile)
	ErrorCopy(err)
	_ = dstFile.Sync()
	srcFile.Close()
	dstFile.Close()
}

//删除文件的函数
func rm(src string) {
	err := os.Remove(src)
	handleError(err, "删除失败")
}

//移动文件函数
func mv(src, dst string) {
	copyFile(src, dst)
	rm(src)
}

//移动文件的另一种写法，但是有些情况又问题
func MV(src, dst string) {
	err := os.Rename(src, dst)
	if err == nil {
		return
	}
	if dst[len(dst)-1] != '/' {
		dst = fmt.Sprintf("%s/", dst)
	}
	mv := func(path string, info os.FileInfo) {
		newPath := dst + info.Name()
		err = os.Rename(path, newPath)
		handleError(err, "移动失败")
	}
	walk(src, mv)
	err = os.Remove(src)
	handleError(err, "删除失败")
}

//处理错误函数
func handleError(err error, msg string) {
	if err != nil {
		log.Printf("%s error: %v", msg, err)
	}
	return
}

//处理错误并退出程序
func handleErrFatal(err error, msg string) {
	if err != nil {
		log.Printf("%s error: %v", msg, err)
		//pause()
		CloseAll()
		os.Exit(1)
	}
	return
}

//ssh客户端
type SSHClient struct {
	sshClient  *ssh.Client
	sftpClient *sftp.Client
}

//获取ssh远程执行命令的返回值
func getCode(err error) int {
	if err == nil {
		return 0
	}
	strS := strings.Split(err.Error(), " ")
	code, _ := strconv.Atoi(strS[len(strS)-1])
	return code
}

//执行命令并打印所有输出
func (sc *SSHClient) getOutPut(cmd string) int {
	session, err := sc.sshClient.NewSession()
	handleError(err, "new session")
	defer session.Close()
	res, err := session.CombinedOutput(cmd)
	fmt.Println(string(res))
	return getCode(err)
}

//执行命令并将输出写入到文件中
func (sc *SSHClient) execCommand(cmd string, writer io.Writer) int {
	sess, err := sc.sshClient.NewSession()
	if err != nil {
		return -1
	}
	handleError(err, "new session")
	//defer sess.Close()
	sess.Stderr = writer
	sess.Stdout = writer
	err = sess.Run(cmd)
	return getCode(err)
}

//执行shll脚本
func (sc *SSHClient) execShellFile(file string, writer io.Writer) int {
	return sc.execFile("", file, writer)
}

//执行任意文件，需要出入执行器或者解释器
func (sc *SSHClient) execFile(executor, file string, writer io.Writer) int {
	var (
		code int
	)
	if executor != "" {
		executor = executor + " "
	}
	tempDir := "/tmp"
	tempFile := file
	fullPath := fmt.Sprintf("%s/%s", tempDir, tempFile)
	sc.upload(file, fullPath)
	code = sc.execCommand(fmt.Sprintf("export TMOUT=0;%s%s", executor, fullPath), writer)
	sc.delRemoteFile(fullPath)
	return code
}

//通过sftp上传文件
func (sc *SSHClient) upload(local, remote string) bool {
	var (
		src            *os.File
		dst            *sftp.File
		err            error
		buffer         []byte
		n              int
		remoteDir      string
		remoteFileName string
	)
	src, err = os.Open(local)
	defer src.Close()
	handleError(err, "打开文件错误")
	if err != nil {
		return false
	}
	remoteDir, remoteFileName = filepath.Split(remote)
	err = sc.sftpClient.MkdirAll(remoteDir)
	handleError(err, "创建文件夹失败")
	if err != nil {
		fmt.Println(remoteFileName)
		return false
	}
	dst, err = sc.sftpClient.Create(remote)
	defer dst.Close()
	handleError(err, "创建远程文件失败")
	if err != nil {
		return false
	}
	buffer = make([]byte, 1024)
	for {
		n, err = src.Read(buffer)
		if n == 0 || err != nil {
			break
		}
		n, _ = dst.Write(buffer[:n])
	}
	_ = dst.Chmod(os.ModePerm)
	fmt.Println("上传成功")
	return true
}

//通过sftp下载文件
func (sc *SSHClient) download(remote, local string) bool {
	var (
		src           *sftp.File
		dst           *os.File
		err           error
		buffer        []byte
		n             int
		localDir      string
		localFileName string
	)
	localDir, localFileName = filepath.Split(local)
	err = os.MkdirAll(localDir, os.ModePerm)
	handleError(err, "创建文件夹失败")
	if err != nil {
		fmt.Println(localFileName)
		return false
	}
	dst, err = os.OpenFile(local, os.O_RDWR, os.ModePerm)
	//defer src.Close()
	handleError(err, "创建本地文件失败")
	if err != nil {
		return false
	}
	src, err = sc.sftpClient.Open(remote)
	defer dst.Close()
	handleError(err, "打开远程文件失败")
	if err != nil {
		return false
	}
	buffer = make([]byte, 1024)
	for {
		n, err = src.Read(buffer)
		if n == 0 || err != nil {
			break
		}
		n, _ = dst.Write(buffer[:n])
		_ = dst.Sync()
	}
	return true
}

//通过sftp删除远端文件
func (sc *SSHClient) delRemoteFile(path string) bool {
	err := sc.sftpClient.Remove(path)
	if err != nil {
		fmt.Println("删除远程文件失败")
		return false
	}
	return true
}

//用于支持ssh连接从键盘输入密码时的自动输入密码
type KeyboardInteractiveChallenge string

//实现处理函数
func (ch KeyboardInteractiveChallenge) keyboardInteractiveChallenge(user, instruction string, questions []string, echos []bool) (answers []string, err error) {
	if len(questions) == 0 {
		return []string{}, nil
	}
	return []string{string(ch)}, nil
}

//让ssh可以一直保持连接，防止断开，通过定时发送消息保持连接
func keepAlive(cl *ssh.Client, done <-chan struct{}) {
	const keepAliveInterval = time.Minute
	t := time.NewTicker(keepAliveInterval)
	defer t.Stop()
	var err error
	for {
		select {
		case <-t.C:
			_, _, err = cl.SendRequest("keepalive@golang.org", true, nil)
			if err != nil {
				handleError(err, "SendRequest")
				return
			}
		case <-done:
			return
		}
	}
}

//创建一个ssh客户端
func newSSH(host, port, user, pwd string, timeout time.Duration) *SSHClient {
	var (
		sftpClient *sftp.Client
	)
	keyboardInteractiveChallenge := func(
		user,
		instruction string,
		questions []string,
		echos []bool,
	) (answers []string, err error) {
		if len(questions) == 0 {
			return []string{}, nil
		}
		return []string{pwd}, nil
	}
	authMethods := []ssh.AuthMethod{ssh.Password(pwd), ssh.KeyboardInteractive(keyboardInteractiveChallenge)}
	authMethods = append(authMethods, ssh.KeyboardInteractive(KeyboardInteractiveChallenge(pwd).keyboardInteractiveChallenge))
	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%s", host, port),
		&ssh.ClientConfig{
			User:            user,
			Auth:            authMethods,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         timeout,
		})
	handleError(err, "Dial")
	if err != nil {
		return nil
	}
	if client == nil {
		return nil
	}
	if sftpClient, err = sftp.NewClient(client); err != nil {
		handleError(err, "sftp连接失败")
		return nil
	}
	C := SSHClient{sshClient: client, sftpClient: sftpClient}
	return &C
}

//从字符串中解析主机信息并创建执行器
func getHostList(name string) []*executor {
	f, err := os.Open(name)
	if err != nil {
		handleError(err, "文件打开错误")
		os.Exit(-1)
	}
	builder := strings.Builder{}
	buffer := make([]byte, 1024)
	n := 0
	for {
		n, err = f.Read(buffer)
		if err != nil {
			break
		}
		builder.Write(buffer[:n])
	}
	content := builder.String()
	lines := strings.Split(content, "\n")
	re := regexp.MustCompile(`[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}`)
	split := regexp.MustCompile(`[, ]+`)
	res := make([]*executor, 0)
	for _, line := range lines {
		line = strings.Trim(line,"\r\n\t ")
		if CONFIG.UserInfo {
			info := split.Split(line, -1)
			if len(info) < 3 {
				continue
			}
			exe := executor{
				host: info[2],
				user: info[0],
				pwd:  info[1],
				cmd:  CONFIG.Cmd,
				file: CONFIG.ShFile,
			}
			res = append(res, &exe)
			continue
		}
		ips := re.FindAllString(line, -1)
		if ips == nil {
			continue
		}
		if len(ips) == 0 {
			continue
		}
		for _, ip := range ips {
			exe := executor{
				host: ip,
				user: CONFIG.User,
				pwd:  CONFIG.Pwd,
				cmd:  CONFIG.Cmd,
				file: CONFIG.ShFile,
			}
			res = append(res, &exe)
		}
	}
	return res
}

//解析配置文件
func parseConfig(filename string) *config {
	file, err := os.Open(filename)
	_, selfName := filepath.Split(os.Args[0])
	handleErrFatal(err, fmt.Sprintf("打开文件失败\n\tusage %s config_file\n", selfName))
	var builder strings.Builder
	buffer := make([]byte, 1024)
	for {
		n, err := file.Read(buffer)
		if err != nil {
			break
		}
		builder.Write(buffer[:n])
	}
	data := builder.String()
	fmt.Println(data)
	args := config{}
	err = json.Unmarshal([]byte(data), &args)
	handleError(err, "Unmarshal")
	fmt.Println("参数:", args)
	if args.Port == 0 {
		args.Port = 22
	}
	if args.Concurrency == 0 {
		args.Concurrency = 10
	}
	return &args

}

//全局控制参数
var (
	ChanFailed      chan ResultInfo //用于接收失败的主机信息
	ChanSuccess     chan ResultInfo //用于接收成功的主机信息
	ChanFinished    chan struct{}   //用于接收已完成的信息
	ChanConcurrency chan struct{}   //用用户控制并发数量，相当于信号量
)

// 全局默认配置参数
var (
	CONFIG               *config //全局配置信息
	ParameterError       = errors.New("参数错误")
	dirName              = "./logs"                           //日志文件夹
	resultDir            = "./logs/.result/"                  //结果保存文件夹
	logTemp              = fmt.Sprintf("#{dirName}/temp")    //缓存文件夹
	successDir           = fmt.Sprintf("%s/success", dirName) // 成功文件夹
	failedDir            = fmt.Sprintf("%s/failed", dirName)  //失败文件夹
	successFile          = resultDir + "success_list.log"     //成功的所有ip保存的文件
	failedFile           = resultDir + "failed_list.log"      //失败的所有ip保存的文件
	FILE_SUCCESS         *MFile                               //成功ip保存文件
	FILE_FAILED          *MFile                               //失败ip保存文件
	PROGRESS             *util.FileMap                        //多进程传递完成进度的共享内存文件
	PROGRESS_MUTEX       util.Semaphore                       //用于阻塞其他进程访问共享内存的互斥信号量
	CMD_OUTPUT_PIPE_NAME = "\\\\.\\Pipe\\SSH_COMMAND_OUTPUT"  //第一个主机执行命令所有输出的命名管道名称
	OutPutNamedPipe      util.NamedPipe                       //第一个主机执行命令所有输出的命名管道文件
)

//初始化通信组件
func initCommunicationComponent() {
	PROGRESS = util.CreateFileMapping("PROGRESS_000", 32)
	PROGRESS_MUTEX = util.CreateSemaphore(0, 1, "PROGRESS_MUTEX")
	OutPutNamedPipe = util.CreateNamedPipe(CMD_OUTPUT_PIPE_NAME, 1, 1024, 1024, 3600)
	OutPutNamedPipe.ConnectNamedPipe() // 等待其他进程连接到输出管道，这是一个阻塞方法
}

func init() {
	runtime.GOMAXPROCS(16)

	ChanFailed = make(chan ResultInfo, 2048)
	ChanSuccess = make(chan ResultInfo, 2048)
	ChanFinished = make(chan struct{}, 4096)

	_ = os.MkdirAll(logTemp, os.ModePerm)
	_ = os.MkdirAll(resultDir, os.ModePerm)
	_ = os.MkdirAll(successDir, os.ModePerm)
	_ = os.MkdirAll(failedDir, os.ModePerm)
	tf, _ := os.Create(successFile)
	FILE_SUCCESS = &MFile{tf}
	ff, _ := os.Create(failedFile)
	FILE_FAILED = &MFile{ff}
}

//全局配置结构
type config struct {
	Cmd         []string
	User        string
	Pwd         string
	Port        int
	HostFile    string
	Concurrency int
	UserInfo    bool
	ShFile      string
}

//export execCommand
func execCommand(host, user, pwd, port string, cmd *string, ) {

}

//关闭所有全局对象
func CloseAll() {
	_, _ = PROGRESS.Write([]byte{1,0})
	PROGRESS_MUTEX.ReleaseSemaphore(1)

	_ = FILE_SUCCESS.Sync()
	_ = FILE_FAILED.Sync()
	_ = FILE_SUCCESS.Close()
	_ = FILE_FAILED.Close()

	_ = PROGRESS.Close()
	_ = OutPutNamedPipe.Close()
	_ = PROGRESS_MUTEX.Close()
	fmt.Println("Closed all")
}

func main() {
	var filename string
	if len(os.Args) < 2 {
		filename = "config.json"
	} else {
		filename = os.Args[1]
	}
	CONFIG = parseConfig(filename)
	//check args
	if CONFIG.UserInfo {
		if CONFIG.User == "" {
			handleErrFatal(ParameterError, "用户名不能为空")
		}
		if CONFIG.Concurrency < 0 {
			handleErrFatal(ParameterError, "并发数必须大于0")
		}
	}
	//初始化通信组件
	initCommunicationComponent()
	defer CloseAll()
	ChanConcurrency = make(chan struct{}, CONFIG.Concurrency)
	count := 1
	ls := getHostList(CONFIG.HostFile)
	if len(ls)==0{
		//handleErrFatal(errors.New("ip为空"),"")
		return
	}
	go ls[0].execute(OutPutNamedPipe, "?", "?")
	for _, exe := range ls[1:] {
		fmt.Println(exe)
		var filename string
		var file string
		var outFile OutFile
		filename = fmt.Sprintf("%s$%s.log", exe.host, time.Now().Format("2006-01-02#15_04_05"))
		file = logTemp + "/" + filename
		outFile, _ = os.Create(file)
		go exe.execute(outFile, file, filename)
		count++
	}
	fmt.Println("count：", count)
	var progress int
	for i := 0; i < count; i++ {
		<-ChanFinished
		progress = (i+1)*10000 / count
		progressStr := fmt.Sprintf("%d", progress)
		data := []byte(progressStr)
		data = append(data, 0)
		_, _ = PROGRESS.Write(data)
		PROGRESS_MUTEX.ReleaseSemaphore(1)
		//fmt.Println("current progress:", progressStr+"%")
	}
	ChanSuccess <- ResultInfo{"ok", ""}
	ChanFailed <- ResultInfo{"ok", ""}
	for {
		i := <-ChanSuccess
		if i.host == "ok" {
			break
		}
		fmt.Println("成功：", i)
	}
	for {
		i := <-ChanFailed
		if i.host == "ok" {
			break
		}
		fmt.Println("失败：", i)
	}
}
