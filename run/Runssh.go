package run

import (
	"fmt"
	"golang.org/x/crypto/ssh"
	"golin/global"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Runssh 通过调用ssh协议执行命令，写入到文件,并减一个线程数
func Runssh(sshname string, sshHost string, sshUser string, sshPasswrod string, sshPort int, cmd string) {
	defer wg.Done()
	// 创建ssh登录配置
	configssh := &ssh.ClientConfig{
		Timeout:         time.Second * 3, // ssh连接timeout时间
		User:            sshUser,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	configssh.Auth = []ssh.AuthMethod{ssh.Password(sshPasswrod)}
	//增加旧版本算法支持,部分机器会出现 ssh: handshake failed: ssh: packet too large 报错
	//configssh.Ciphers = []string{"aes128-cbc", "aes256-cbc", "3des-cbc", "aes128-ctr", "aes192-ctr", "aes256-ctr", "aes128-gcm@openssh.com", "chacha20-poly1305@openssh.com"}

	// dial 获取ssh client
	addr := fmt.Sprintf("%s:%d", sshHost, sshPort)
	sshClient, err := ssh.Dial("tcp", addr, configssh)
	if err != nil {
		errhost = append(errhost, sshHost)
		//fmt.Println(err)
		return
	}
	defer sshClient.Close()

	// 创建ssh-session
	session, err := sshClient.NewSession()
	defer session.Close()
	if err != nil {
		errhost = append(errhost, sshHost)
		return
	}

	firepath := filepath.Join(succpath, "Linux")

	// 自定义命令存在则只执行自定义文件
	if runcmd != "" {
		combo, err := session.CombinedOutput(cmd)
		if err != nil {
			errhost = append(errhost, sshHost)
			return
		}

		//判断是否进行输出命令结果
		if echorun {
			fmt.Printf("%s\n%s\n", "<输出结果>", string(combo))
		}
		datanew := []byte(string(combo))
		err = os.WriteFile(filepath.Join(firepath, fmt.Sprintf("%s_%s.log", sshname, sshHost)), datanew, fs.FileMode(global.FilePer))
		if err != nil {
			errhost = append(errhost, sshHost)
			return
		}
		return
	}

	// 执行模板文件
	data := Data{
		Name: fmt.Sprintf("%s_%s", sshname, sshHost),
		Info: ServerInfo{
			HostName:    runCmd("hostname", sshClient),
			Arch:        runCmd("arch", sshClient),
			Cpu:         runCmd(`cat /proc/cpuinfo | grep name | sort | uniq|awk -F ":" '{print $2}'| xargs`, sshClient),
			CpuPhysical: runCmd(`cat /proc/cpuinfo |grep "physical id"|sort|uniq| wc -l`, sshClient),
			CpuCore:     runCmd(`cat /proc/cpuinfo | grep "core id" | sort | uniq | wc -l`, sshClient),
			Version:     runCmd("rpm -q centos-release", sshClient),
			ProductName: runCmd(`dmidecode -t system | grep 'Product Name'|awk -F ":" '{print $2}'|xargs `, sshClient),
			Free:        runCmd(`free -g | grep Mem | awk '{print $2}'`, sshClient),
		},
		Address:      runCmd("ifconfig", sshClient),
		Disk:         runCmd("df -h", sshClient),
		PamSSH:       runCmd(`cat /etc/pam.d/sshd|grep -v "^#"|grep -v "^$"`, sshClient),
		PamPasswd:    runCmd(`cat /etc/pam.d/passwd|grep -v "^#"|grep -v "^$"`, sshClient),
		IptablesInfo: runCmd("iptables -L", sshClient),
		PS:           runCmd("ps aux", sshClient),
		Sudoers:      runCmd(`cat /etc/sudoers|grep -v "^#"|grep -v "^$"`, sshClient),
		Rsyslog:      runCmd(`cat /etc/rsyslog.conf|grep -v "^#"|grep -v "^$"`, sshClient),
		CronTab:      runCmd(`cat /etc/crontab|grep -v "^#"|grep -v "^$";crontab -l`, sshClient),
		Share:        runCmd(`cat /etc/exports|grep -v "^#"|grep -v "^$"`, sshClient),
		Env:          runCmd(`cat /etc/profile |grep -v "^#" |grep -v "^$"`, sshClient),
		Version:      runCmd("cat /etc/*release;uname -r", sshClient),
		Docker:       runCmd("docker ps", sshClient),
		ListUnit:     runCmd("systemctl list-unit-files|grep enabled", sshClient),
		HeadLog:      runCmd(`head -n 10 /var/log/messages /var/log/secure /var/log/audit/audit.log  /var/log/yum.log /var/log/cron`, sshClient),
		TailLog:      runCmd(`tail -n 10 /var/log/messages /var/log/secure /var/log/audit/audit.log  /var/log/yum.log /var/log/cron`, sshClient),
		User:         make([]LinUser, 0),
		CreateUser:   make([]Logindefs, 0),
		Port:         make([]PortList, 0),
		ConfigSSH:    make([]SSH, 0),
		FilePer:      make([]FileListPer, 0),
		FireWalld:    make([]FireListWalld, 0),
	}

	// 通过/etc/passwd 以及结合chage命令获取用户基本信息
	for _, v := range strings.Split(runCmd("cat /etc/passwd", sshClient), "\n") {
		userinfo := strings.Split(v, ":")
		if len(userinfo) != 7 {
			continue
		}
		var Login bool
		if userinfo[6] == "/bin/bash" || userinfo[6] == "/bin/zsh" {
			Login = true
		}
		shadow := strings.Split(runCmd(fmt.Sprintf("chage -l %s", userinfo[0]), sshClient), "\n")
		user := LinUser{
			Name:          userinfo[0],
			Passwd:        userinfo[1],
			Uid:           userinfo[2],
			Gid:           userinfo[3],
			Description:   userinfo[4],
			Pwd:           userinfo[5],
			Bash:          userinfo[6],
			Login:         Login,
			LastPasswd:    strings.Split(shadow[0], ":")[1],
			PasswdExpired: strings.Split(shadow[1], ":")[1],
			Lose:          strings.Split(shadow[2], ":")[1],
			UserExpired:   strings.Split(shadow[3], ":")[1],
			MaxPasswd:     strings.Split(shadow[5], ":")[1],
		}
		data.User = append(data.User, user)
	}

	//读取/etc/login.defs获取新创建用户时的信息
	CreateUserLogindefs := Logindefs{
		PassMaxDays:   runCmd(`cat /etc/login.defs |grep -v "^#" |grep "PASS_MAX_DAYS"|awk -F " " '{print $2}'`, sshClient),
		PassMinDays:   runCmd(`cat /etc/login.defs |grep -v "^#" |grep "PASS_MIN_DAYS"|awk -F " " '{print $2}'`, sshClient),
		PassWarnAge:   runCmd(`cat /etc/login.defs |grep -v "^#" |grep "PASS_WARN_AGE"|awk -F " " '{print $2}'`, sshClient),
		UMASK:         runCmd(`cat /etc/login.defs |grep -v "^#" |grep "UMASK"|awk -F " " '{print $2}'`, sshClient),
		EncryptMethod: runCmd(`cat /etc/login.defs |grep -v "^#" |grep "ENCRYPT_METHOD"|awk -F " " '{print $2}'`, sshClient),
	}
	data.CreateUser = append(data.CreateUser, CreateUserLogindefs)

	//通过ss -tulnp获取端口信息
	for _, p := range strings.Split(runCmd(`ss -tulnp|grep -v "Netid"`, sshClient), "\n") {
		re := regexp.MustCompile(`\s+`)
		s := re.ReplaceAllString(p, " ")
		listen := strings.Split(s, " ")
		if len(listen) != 7 {
			continue
		}
		listenPort := PortList{
			Netid:   listen[0],
			State:   listen[1],
			Local:   listen[4],
			Process: listen[6],
		}
		data.Port = append(data.Port, listenPort)
	}

	//获取sshd_config配置
	sshdconfig := NewSSH()
	if runCmd(`cat /etc/ssh/sshd_config|grep -v "^#" |grep "PasswordAuthentication"|awk -F " " '{print $2}'`, sshClient) == "no" {
		sshdconfig.PasswordAuthentication = false
	}
	if runCmd(`cat /etc/ssh/sshd_config|grep -v "^#" |grep "PermitRootLogin"|awk -F " " '{print $2}'`, sshClient) == "no" {
		sshdconfig.PermitRootLogin = false
	}
	if runCmd(`cat /etc/ssh/sshd_config|grep -v "^#" |grep "PermitEmptyPasswords"|awk -F " " '{print $2}'`, sshClient) == "yes" {
		sshdconfig.PermitEmptyPasswords = true
	}
	if runCmd(`cat /etc/ssh/sshd_config|grep -v "^#" |grep "Protocol"|awk -F " " '{print $2}'`, sshClient) != "" {
		sshdconfig.Protocol = runCmd(`cat /etc/ssh/sshd_config|grep -v "^#" |grep "Protocol"|awk -F " " '{print $2}'`, sshClient)
	}
	if runCmd(`cat /etc/ssh/sshd_config|grep -v "^#" |grep "MaxAuthTries"|awk -F " " '{print $2}'`, sshClient) != "" {
		sshdconfig.MaxAuthTries = runCmd(`cat /etc/ssh/sshd_config|grep -v "^#" |grep "MaxAuthTries"|awk -F " " '{print $2}'`, sshClient)
	}
	data.ConfigSSH = append(data.ConfigSSH, sshdconfig)

	//读取地址限制
	data.HostAllow = runCmd(` cat /etc/hosts.allow |grep -v "^#" |grep -v "^$"`, sshClient)
	data.HostDeny = runCmd(` cat /etc/hosts.Deny |grep -v "^#" |grep -v "^$"`, sshClient)

	//中文文件权限
	var FileList = []string{"/etc/passwd", "/etc/shadow", "/etc/group", "/etc/rsyslog.conf", "/etc/sudoers", "/etc/hosts.allow", "/etc/hosts.deny", "/etc/ssh/sshd_config", "/etc/pam.d/sshd", "/etc/pam.d/passwd", "/var/log/messages", "/var/log/audit/audit.log"}
	for _, name := range FileList {
		FilePer := FileListPer{
			Name:          name,
			Permission:    runCmd(fmt.Sprintf("stat -c '%s' %s ", "%a", name), sshClient),
			Size:          runCmd(fmt.Sprintf("stat -c '%s' %s ", "%s", name), sshClient),
			Uid:           runCmd(fmt.Sprintf("stat -c '%s' %s ", "%U", name), sshClient),
			Gid:           runCmd(fmt.Sprintf("stat -c '%s' %s ", "%G", name), sshClient),
			LastReadTime:  runCmd(fmt.Sprintf("stat -c '%s' %s ", "%x", name), sshClient),
			LastWriteTime: runCmd(fmt.Sprintf("stat -c '%s' %s ", "%y", name), sshClient),
		}
		data.FilePer = append(data.FilePer, FilePer)
	}

	//防火墙 selinux状态
	data.FireWalld = append(data.FireWalld, FireListWalld{
		Name:   "firewalld",
		Status: runCmd(fmt.Sprintf(`systemctl status firewalld  |grep "Active"|awk -F " " '{print $2}'`), sshClient),
	})
	data.FireWalld = append(data.FireWalld, FireListWalld{
		Name:   "selinux",
		Status: runCmd(fmt.Sprintf(`cat /etc/selinux/config |grep -v "^#"|grep -v "^$"|awk -F 'SELINUX=' '{print $2}'`), sshClient),
	})

	// 读取模板文件
	tmpl, err := template.ParseFS(templateFile, "linux_html.html")
	if err != nil {
		errhost = append(errhost, sshHost)
		return
	}
	// 创建一个新的文件
	newFile, err := os.Create(fmt.Sprintf("%s/%s_%s.html", firepath, sshname, sshHost))
	if err != nil {
		errhost = append(errhost, sshHost)
		return
	}
	defer newFile.Close()
	// 将模板执行的结果写入新的文件
	err = tmpl.Execute(newFile, data)
	if err != nil {
		errhost = append(errhost, sshHost)
		return
	}

}

func runCmd(cmd string, Client *ssh.Client) string {
	newClient, err := Client.NewSession()
	if err != nil {
		fmt.Println(err)
		return ""
	}
	defer newClient.Close()
	combo, err := newClient.CombinedOutput(cmd)
	if err != nil {
		return ""
	}
	return string(combo)
}
