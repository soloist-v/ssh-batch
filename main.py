import json
import os
import time
from threading import Thread
from json import dumps
from subprocess import call, Popen, STDOUT
from PyQt5 import QtCore, QtWidgets
from PyQt5.QtCore import pyqtSignal, QStringListModel, QModelIndex
from PyQt5.QtWidgets import QApplication, QMainWindow, QFileDialog
import shutil
import datetime
import ctypes
import sys

if hasattr(sys, "frozen"):
    parent_dir = os.path.dirname(sys.executable)
else:
    parent_dir = os.path.dirname(__file__)
sys.stdout = open("debug0.log", "w")

FILE_MAP_READ = 0x04
FILE_MAP_WRITE = 0x02
INFINITE = 0xFFFFFFFF
TIMEOUT = 258
kernel32 = ctypes.windll.LoadLibrary("kernel32.dll")
OpenFileMapping = kernel32.OpenFileMappingA
MapViewOfFile = kernel32.MapViewOfFile
MapViewOfFile.restype = ctypes.POINTER(ctypes.c_byte)
OpenSemaphoreA = kernel32.OpenSemaphoreA
CreateSemaphore = kernel32.CreateSemaphoreA
CloseHandle = kernel32.CloseHandle
WaitForSingleObject = kernel32.WaitForSingleObject
ReleaseSemaphore = kernel32.ReleaseSemaphore
UnmapViewOfFile = kernel32.UnmapViewOfFile

config_name = "temp_config.json"
host_file = "temp_host.ls"
progress_file_mapping_name = "PROGRESS_000"
ssh_output_pipe_name = "\\\\.\\Pipe\\SSH_COMMAND_OUTPUT"
ssh_batch_exe_debug_log_file = open("debug.log", 'wb')


def print(*args):
    for arg in args:
        sys.stdout.write("%s\n" % arg)
        sys.stdout.flush()


def kill_proc(pid):
    call(["taskkill.exe", "/f", "/pid", str(pid)])


class History:
    def __init__(self, name=".history", **kwargs):
        for k, v in kwargs.items():
            setattr(self, k, v)
        if os.path.isfile(name):
            with open(name, 'r') as f:
                content = f.read()
            try:
                data = json.loads(content)
            except:
                data = {}
            self.set_by_dict(data)
            if data != {}:
                kwargs = data
        self.keys = list(kwargs.keys())

    def set_by_dict(self, d):
        self.__dict__.update(d)

    def save(self):
        self.__dict__.copy()
        data = {}
        for k in self.keys:
            data[k] = self.__dict__[k]
        res = json.dumps(data)
        with open(".history", 'wb') as f:
            f.write(res.encode("utf8"))


import subprocess


def popen(cmd: str):
    """For pyinstaller -w"""
    startupinfo = subprocess.STARTUPINFO()
    startupinfo.dwFlags |= subprocess.STARTF_USESHOWWINDOW
    process = subprocess.Popen(cmd, startupinfo=startupinfo, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                               stdin=subprocess.PIPE)
    return process


class QTextEdit(QtWidgets.QTextEdit):

    def insertFromMimeData(self, QMimeData):
        if QMimeData.hasText():
            self.textCursor().insertText(QMimeData.text())  # 去除粘贴格式


class MainWindow(QMainWindow):
    set_out = pyqtSignal(str, name='set_out')
    set_progress = pyqtSignal(int, name='set_progress')

    def __init__(self, history):
        super(MainWindow, self).__init__()
        self.cur_stop_event_func = None
        self.history = history

        self.setObjectName("MainWindow")
        self.setFixedHeight(837)
        self.setFixedWidth(770)
        self.setStyleSheet("")
        self.centralwidget = QtWidgets.QWidget(self)
        self.centralwidget.setObjectName("centralwidget")
        self.btn_choose_file = QtWidgets.QPushButton(self.centralwidget)
        self.btn_choose_file.setGeometry(QtCore.QRect(660, 30, 96, 51))
        self.btn_choose_file.setStyleSheet(");")
        self.btn_choose_file.setObjectName("btn_choose_file")
        self.te_host = QTextEdit(self.centralwidget)
        self.te_host.setGeometry(QtCore.QRect(70, 10, 581, 91))
        self.te_host.setObjectName("te_host")
        self.lb_host_list = QtWidgets.QLabel(self.centralwidget)
        self.lb_host_list.setGeometry(QtCore.QRect(10, 20, 61, 71))
        self.lb_host_list.setStyleSheet("")
        self.lb_host_list.setObjectName("lb_host_list")
        self.label_2 = QtWidgets.QLabel(self.centralwidget)
        self.label_2.setGeometry(QtCore.QRect(10, 250, 61, 21))
        self.label_2.setStyleSheet("")
        self.label_2.setObjectName("label_2")
        self.te_user = QtWidgets.QLineEdit(self.centralwidget)
        self.te_user.setGeometry(QtCore.QRect(72, 249, 121, 21))
        self.te_user.setStyleSheet("")
        self.te_user.setObjectName("te_user")
        self.label_3 = QtWidgets.QLabel(self.centralwidget)
        self.label_3.setGeometry(QtCore.QRect(210, 250, 41, 21))
        self.label_3.setStyleSheet("")
        self.label_3.setObjectName("label_3")
        self.te_pwd = QtWidgets.QLineEdit(self.centralwidget)
        self.te_pwd.setGeometry(QtCore.QRect(250, 250, 121, 21))
        self.te_pwd.setStyleSheet("")
        self.te_pwd.setObjectName("te_pwd")
        self.label_4 = QtWidgets.QLabel(self.centralwidget)
        self.label_4.setGeometry(QtCore.QRect(400, 250, 41, 21))
        self.label_4.setStyleSheet("")
        self.label_4.setObjectName("label_4")
        self.te_port = QtWidgets.QLineEdit(self.centralwidget)
        self.te_port.setGeometry(QtCore.QRect(440, 250, 41, 21))
        self.te_port.setStyleSheet("")
        self.te_port.setObjectName("te_port")
        self.label_5 = QtWidgets.QLabel(self.centralwidget)
        self.label_5.setGeometry(QtCore.QRect(510, 250, 91, 21))
        self.label_5.setStyleSheet("")
        self.label_5.setObjectName("label_5")
        self.te_concurrency = QtWidgets.QLineEdit(self.centralwidget)
        self.te_concurrency.setGeometry(QtCore.QRect(610, 250, 41, 21))
        self.te_concurrency.setStyleSheet("")
        self.te_concurrency.setObjectName("te_concurrency")
        self.label_6 = QtWidgets.QLabel(self.centralwidget)
        self.label_6.setGeometry(QtCore.QRect(10, 290, 181, 21))
        self.label_6.setStyleSheet("")
        self.label_6.setObjectName("label_6")
        self.rb_yes = QtWidgets.QRadioButton(self.centralwidget)
        self.rb_yes.setGeometry(QtCore.QRect(200, 290, 41, 21))
        self.rb_yes.setStyleSheet("color: rgb(255, 165, 171);")
        self.rb_yes.setObjectName("rb_yes")
        self.rb_no = QtWidgets.QRadioButton(self.centralwidget)
        self.rb_no.setGeometry(QtCore.QRect(250, 290, 41, 21))
        self.rb_no.setStyleSheet("color: rgb(37, 102, 255);")
        self.rb_no.setChecked(True)
        self.rb_no.setObjectName("rb_no")
        self.progressBar = QtWidgets.QProgressBar(self.centralwidget)
        self.progressBar.setGeometry(QtCore.QRect(10, 810, 761, 20))
        self.progressBar.setStyleSheet("")
        self.progressBar.setProperty("value", 0)
        self.progressBar.setObjectName("progressBar")
        self.out = QtWidgets.QTextBrowser(self.centralwidget)
        self.out.setGeometry(QtCore.QRect(10, 320, 431, 481))
        self.out.setAutoFillBackground(False)
        self.out.setStyleSheet("background-color: rgb(0, 0, 0);\n"
                               "color: rgb(255, 255, 255);")
        self.out.setObjectName("out")
        self.lv_success = QtWidgets.QListView(self.centralwidget)
        self.lv_success.setGeometry(QtCore.QRect(450, 320, 141, 481))
        self.lv_success.setStyleSheet("")
        self.lv_success.setObjectName("lv_success")
        self.lv_failed = QtWidgets.QListView(self.centralwidget)
        self.lv_failed.setGeometry(QtCore.QRect(600, 320, 141, 481))
        self.lv_failed.setStyleSheet("")
        self.lv_failed.setObjectName("lv_failed")
        self.label_7 = QtWidgets.QLabel(self.centralwidget)
        self.label_7.setGeometry(QtCore.QRect(490, 292, 54, 20))
        self.label_7.setStyleSheet("")
        self.label_7.setObjectName("label_7")
        self.label_8 = QtWidgets.QLabel(self.centralwidget)
        self.label_8.setGeometry(QtCore.QRect(630, 290, 54, 20))
        self.label_8.setStyleSheet("")
        self.label_8.setObjectName("label_8")
        self.btn_start = QtWidgets.QPushButton(self.centralwidget)
        self.btn_start.setGeometry(QtCore.QRect(670, 240, 75, 41))
        self.btn_start.setStyleSheet("")
        self.btn_start.setObjectName("btn_start")
        self.lb_host_list_2 = QtWidgets.QLabel(self.centralwidget)
        self.lb_host_list_2.setGeometry(QtCore.QRect(10, 140, 61, 71))
        self.lb_host_list_2.setStyleSheet("")
        self.lb_host_list_2.setObjectName("lb_host_list_2")
        self.te_cmd = QTextEdit(self.centralwidget)
        self.te_cmd.setGeometry(QtCore.QRect(70, 130, 581, 91))
        self.te_cmd.setObjectName("te_cmd")
        self.btn_choose_sh = QtWidgets.QPushButton(self.centralwidget)
        self.btn_choose_sh.setGeometry(QtCore.QRect(660, 150, 96, 51))
        self.btn_choose_sh.setStyleSheet("")
        self.btn_choose_sh.setObjectName("btn_choose_sh")
        self.setCentralWidget(self.centralwidget)
        self.re_translate_ui()
        QtCore.QMetaObject.connectSlotsByName(self)

        self.name_map = {}
        self.success_ls_model = QStringListModel()
        self.failed_ls_model = QStringListModel()
        self.set_click_events()
        self.set_out.connect(self.add_out)
        self.out.setLineWrapMode(QtWidgets.QTextEdit.NoWrap)
        self.set_progress.connect(self.progressBar.setValue)

    def add_out(self, text):
        self.out.append(text)
        self.out.moveCursor(self.out.textCursor().End)

    @staticmethod
    def generate_config(host_str, cmd, user, pwd, port, concurrency_num, is_parse):
        print("generate_config")
        with open(host_file, "w") as f:
            f.write(host_str)
        sh_file = "tmp.sh"
        with open("tmp.sh", "wb") as f:
            f.write(cmd.encode("utf8", errors="ignore"))
        config = {
            "cmd": [
                ""
            ],
            "user": user,
            "pwd": pwd,
            "port": int(port),
            "hostFile": host_file,
            "concurrency": int(concurrency_num),
            "UserInfo": is_parse,
            "ShFile": sh_file
        }
        with open(config_name, 'w')as f:
            f.write(dumps(config))
        return config_name

    @staticmethod
    def backup_old_record():
        os.makedirs("history", exist_ok=True)
        now = datetime.datetime.now()
        name = "%s-%s-%s_%s-%s-%s" % (now.year, now.month, now.day, now.hour, now.minute, now.second)
        print(name)
        if os.path.exists("logs"):
            shutil.move('logs', "history/%s" % name)

    def start(self):
        # backup old logs
        self.backup_old_record()
        # clear
        self.name_map = {}
        self.lv_success.setModel(QStringListModel())
        self.lv_failed.setModel(QStringListModel())
        self.progressBar.setValue(0)
        self.out.setText("")
        # read args
        host_str = self.te_host.toPlainText()
        cmd_str = self.te_cmd.toPlainText()
        user = self.te_user.text()
        pwd = self.te_pwd.text()
        port = self.te_port.text()
        concurrency_num = self.te_concurrency.text()
        if self.rb_yes.isChecked():
            is_parse_user = True
        if self.rb_no.isChecked():
            is_parse_user = False
        cur_config_name = self.generate_config(host_str, cmd_str, user, pwd, port, concurrency_num, is_parse_user)
        try:
            proc = Popen(["ssh_batch.exe", cur_config_name], creationflags=0x08000000, stdin=-1, stderr=STDOUT,
                         stdout=ssh_batch_exe_debug_log_file)
        except:
            # 检测如果ssh_batch.exe 进程是否还在内存中，如果在就关掉，否则共享内存、信号量、命名管道不能创建成功
            for p in psutil.process_iter():
                if p.name().lower() == "ssh_batch.exe":
                    p.terminal()
        print("proc create success")
        Thread(target=self.show_progress).start()
        Thread(target=self.fetch_cmd_out, args=(proc,)).start()
        self.btn_start.setText("stop")
        self.btn_start.setStyleSheet("background-color: rgb(0, 0, 0);\n"
                                     "color: rgb(255, 255, 255);")
        self.cur_stop_event_func = lambda: kill_proc(proc.pid)
        self.btn_start.clicked.disconnect(self.start)
        self.btn_start.clicked.connect(self.cur_stop_event_func)

    def fetch_cmd_out(self, proc):
        # 等待管道创建完成并等待就绪后才能连接到管道
        for i in range(50):
            try:
                time.sleep(0.1)
                ssh_out_file = open(ssh_output_pipe_name, "rb")
            except:
                continue
            break
        else:
            print("获取输出失败")
            return
        while 1:
            data = os.read(ssh_out_file.fileno(), 1024)
            if not data:
                break
            self.set_out.emit(data.decode("utf8", errors="ignore"))
        ret_code = proc.wait()
        print("proc exit %s" % ret_code)
        self.on_finished()

    def show_progress(self):
        # 利用共享内存获取当前进度
        print("show progress")
        name = ctypes.c_char_p(progress_file_mapping_name.encode("utf8"))
        # 必须等待共享内存创建完成才能开始后续读取
        for i in range(100):
            time.sleep(0.1)
            handle_file_mapping = OpenFileMapping(FILE_MAP_READ | FILE_MAP_WRITE, 0, name)
            if handle_file_mapping != 0:
                break
        else:
            self.btn_start.setText("启动失败")
            return
        p_buffer = MapViewOfFile(handle_file_mapping, FILE_MAP_READ | FILE_MAP_WRITE, 0, 0, 0)
        semaphore = OpenSemaphoreA(2031619, None, ctypes.c_char_p(b"PROGRESS_MUTEX"))
        print("MapViewOfFile", p_buffer, "semaphore", semaphore)
        final = 10000
        while 1:
            if WaitForSingleObject(semaphore, INFINITE) == TIMEOUT:
                print("超时错误")
                break
            res = p_buffer[:32]
            if res[0] == 1:
                if res[3] == 0:
                    final = 0
                break
            index_zero = res.index(0)
            progress = bytes(res[:index_zero])
            print(progress)
            # progress = res.rstrip(b"\0")
            if progress.isdigit():
                self.set_progress.emit(int(progress))
                self.update_success_list()
                self.update_failed_list()

        print("exit thread.....")
        self.set_progress.emit(int(final))
        self.update_success_list()
        self.update_failed_list()
        UnmapViewOfFile(p_buffer)
        CloseHandle(semaphore)
        CloseHandle(handle_file_mapping)

    def on_finished(self):
        self.btn_start.setText("start")
        self.btn_start.setStyleSheet("")
        self.btn_start.clicked.disconnect(self.cur_stop_event_func)
        self.btn_start.clicked.connect(self.start)
        self.update_failed_list()
        self.update_success_list()

    def choose_file(self, msg="", directory="./"):
        names = QFileDialog.getOpenFileName(self, msg, directory)
        file_path = names[0]
        return file_path

    def choose_host(self):
        file_path = self.choose_file("选择host文件", self.history.host_file)
        if file_path != "":
            self.history.host_file = os.path.split(file_path)[0]
        if not os.path.exists(file_path):
            return
        with open(file_path, 'rb') as f:
            data = f.read()
        self.te_host.setText(data.decode("utf8", errors="ignore"))

    def choose_sh(self):
        file_path = self.choose_file("选择sh脚本", self.history.sh_file)
        if file_path != "":
            self.history.sh_file = os.path.split(file_path)[0]
        if not os.path.exists(file_path):
            return
        with open(file_path, 'rb') as f:
            data = f.read()
        self.te_cmd.setText(data.decode("utf8", errors="ignore"))

    def set_click_events(self):
        self.btn_start.clicked.connect(self.start)
        self.btn_choose_file.clicked.connect(self.choose_host)
        self.btn_choose_sh.clicked.connect(self.choose_sh)
        self.lv_success.doubleClicked.connect(self.double_success)
        self.lv_failed.doubleClicked.connect(self.double_failed)

    def double_success(self, index: QModelIndex):
        path = index.data()
        Popen(["explorer", "logs\\success\\" + self.name_map[path]])

    def double_failed(self, index: QModelIndex):
        path = index.data()
        Popen(["explorer", "logs\\failed\\" + self.name_map[path]])

    def update_success_list(self):
        success_list = os.listdir("./logs/success")
        if len(success_list) == 0:
            return
        ip_list = []
        for i in success_list:
            ip = i.split("$")[0]
            self.name_map[ip] = i
            ip_list.append(ip)
        self.success_ls_model.setStringList(ip_list)
        self.lv_success.setModel(self.success_ls_model)

    def update_failed_list(self):
        failed_list = os.listdir("./logs/failed")
        if len(failed_list) == 0:
            return
        ip_list = []
        for i in failed_list:
            ip = i.split("$")[0]
            self.name_map[ip] = i
            ip_list.append(ip)
        self.failed_ls_model.setStringList(ip_list)
        self.lv_failed.setModel(self.failed_ls_model)

    def re_translate_ui(self):
        _translate = QtCore.QCoreApplication.translate
        self.setWindowTitle(_translate("MainWindow", "SSH批量执行工具"))
        self.btn_choose_file.setText(_translate("MainWindow", "选择列表文件"))
        self.lb_host_list.setText(_translate("MainWindow", "host列表"))
        self.label_2.setText(_translate("MainWindow", "用户名："))
        self.te_user.setText(_translate("MainWindow", "root"))
        self.label_3.setText(_translate("MainWindow", "密码："))
        self.te_pwd.setText(_translate("MainWindow", "123456"))
        self.label_4.setText(_translate("MainWindow", "端口："))
        self.te_port.setText(_translate("MainWindow", "22"))
        self.label_5.setText(_translate("MainWindow", "同时最大连接数："))
        self.te_concurrency.setText(_translate("MainWindow", "40"))
        self.label_6.setText(_translate("MainWindow", "是否从列表中解析用户名和密码："))
        self.rb_yes.setText(_translate("MainWindow", "是"))
        self.rb_no.setText(_translate("MainWindow", "否"))
        self.out.setPlaceholderText(_translate("MainWindow", "输出："))
        self.label_7.setText(_translate("MainWindow", "成功项"))
        self.label_8.setText(_translate("MainWindow", "失败项"))
        self.btn_start.setText(_translate("MainWindow", "start"))
        self.lb_host_list_2.setText(_translate("MainWindow", "命令："))
        self.progressBar.setMaximum(10000)
        self.btn_choose_sh.setText(_translate("MainWindow", "选择脚本文件"))


if __name__ == '__main__':
    import sys
    import psutil

    his = History(host_file="./", sh_file='./')
    app = QApplication(sys.argv)
    win = MainWindow(his)
    win.show()
    code = app.exec_()
    his.save()
    ssh_batch_exe_debug_log_file.close()
    sys.stdout.close()
    sys.exit(code)
