@echo off

REM 确保 Go 已安装并添加到系统路径中
where go >nul 2>nul
if errorlevel 1 (
    echo Go 未安装或未添加到系统路径中
    exit /b 1
)

REM 构建 main.go 文件
go build -o main.exe

REM 设置窗口标题
title Raft Cluster Startup

REM 启动第一组节点
start "Master Node1:8001" main.exe -httpport 8001 -raftport 9001 -node node1 -bootstrap true
start "Master Node2:8002" main.exe -httpport 8002 -raftport 9002 -node node2 -bootstrap true 
start "Master Node3:8003" main.exe -httpport 8003 -raftport 9003 -node node3 -bootstrap true

REM 等待第一组节点启动
timeout /t 5 > nul

REM 启动第二组节点
start "Slave Node11:8004" main.exe -httpport 8004 -raftport 9004 -node node11 -joinaddr 127.0.0.1:8001
start "Slave Node12:8005" main.exe -httpport 8005 -raftport 9005 -node node12 -joinaddr 127.0.0.1:8001

REM 等待第二组节点启动
timeout /t 5 > nul 

REM 启动第三组节点
start "Slave Node21:8006" main.exe -httpport 8006 -raftport 9006 -node node21 -joinaddr 127.0.0.1:8002
start "Slave Node22:8007" main.exe -httpport 8007 -raftport 9007 -node node22 -joinaddr 127.0.0.1:8002

REM 等待第三组节点启动
timeout /t 5 > nul

REM 启动第四组节点
start "Slave Node31:8008" main.exe -httpport 8008 -raftport 9008 -node node31 -joinaddr 127.0.0.1:8003
start "Slave Node32:8009" main.exe -httpport 8009 -raftport 9009 -node node32 -joinaddr 127.0.0.1:8003

REM 等待所有节点启动
timeout /t 5 > nul

REM 暂停以便观察输出
pause