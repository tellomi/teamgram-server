"""Original Tellomi Chinese translations for client string catalogs.

Placeholders (%@, %1$s, {name}) are preserved. The app name Tellomi is kept.
Traditional text is OpenCC s2twp from the simplified translation, which is the
wording used for zh-hant (訊息、設定、檔案) rather than a second machine translation.
"""

from __future__ import annotations

import json
import re
from collections import Counter
from pathlib import Path

import opencc

_OPENCC = opencc.OpenCC("s2twp")

_TOKEN = re.compile(
    r"https?://[^\s<>()]+|t\.me/[^\s<>()]+|www\.[^\s<>()]+"
    r"|%%|%(?:\d+\$)?\d*[dfsiu@]|%(?:\d+\$)?\.\d+f|\{[A-Za-z0-9_]+\}|\bun\d+\b"
)

# Words and short phrases. Longer keys win. Lookup is lowercase.
_PHRASES_RAW = r"""
please confirm your country code and enter your phone number|请确认你的国家代码并输入手机号码。
you can change your language later in settings|你可以稍后在设置中更改语言。
choose your language|选择语言
continue in english|使用英语继续
send message|发送消息
send messages|发送消息
new message|新消息
voice message|语音消息
video message|视频消息
secret chat|私密聊天
new secret chat|新建私密聊天
saved messages|收藏夹
phone number|手机号码
country code|国家代码
first name|名字
last name|姓氏
user name|用户名
username|用户名
two-step verification|两步验证
2-step verification|两步验证
data and storage|数据和存储
notifications and sounds|通知和声音
privacy and security|隐私与安全
blocked users|已屏蔽的用户
active sessions|活动会话
devices|设备
add contact|添加联系人
add contacts|添加联系人
invite friends|邀请好友
share contact|分享联系人
edit message|编辑消息
delete message|删除消息
delete messages|删除消息
delete chat|删除聊天
delete chats|删除聊天
clear history|清空历史记录
clear cache|清除缓存
mark as read|标为已读
mark as unread|标为未读
pin to top|置顶
unpin|取消置顶
mute|静音
unmute|取消静音
archive|归档
unarchive|取消归档
forward|转发
reply|回复
copy|复制
paste|粘贴
select|选择
select all|全选
deselect|取消选择
search|搜索
cancel|取消
done|完成
save|保存
edit|编辑
delete|删除
remove|移除
create|创建
continue|继续
next|下一步
back|返回
skip|跳过
close|关闭
open|打开
share|分享
send|发送
call|通话
video call|视频通话
voice call|语音通话
settings|设置
profile|个人资料
contacts|联系人
chats|聊天
chat|聊天
groups|群组
group|群组
channels|频道
channel|频道
stories|动态
story|动态
calls|通话
stickers|贴纸
sticker|贴纸
emoji|表情
emojis|表情
gif|GIF
gifs|GIF
poll|投票
quiz|问答
location|位置
live location|实时位置
contact|联系人
document|文件
file|文件
files|文件
photo|图片
photos|图片
video|视频
videos|视频
audio|音频
music|音乐
message|消息
messages|消息
notification|通知
notifications|通知
privacy|隐私
security|安全
storage|存储
network|网络
proxy|代理
language|语言
theme|主题
wallpaper|壁纸
appearance|外观
night mode|夜间模式
dark mode|深色模式
power saving|省电模式
passcode|锁定码
fingerprint|指纹
face id|面容
password|密码
email|电子邮箱
phone|手机
log out|退出登录
log in|登录
sign up|注册
sign in|登录
account|账号
accounts|账号
profile photo|头像
set profile photo|设置头像
bio|简介
about|关于
status|状态
online|在线
offline|离线
last seen|最后上线
recently|最近
within a week|一周内
within a month|一个月内
long time ago|很久以前
typing|正在输入
recording voice|正在录音
sending photo|正在发送图片
members|成员
member|成员
subscribers|订阅者
subscriber|订阅者
admins|管理员
admin|管理员
owner|所有者
banned|已封禁
restricted|受限制
permissions|权限
exceptions|例外
everybody|所有人
my contacts|我的联系人
nobody|没有人
always allow|始终允许
never allow|从不允许
add|添加
added|已添加
invite|邀请
invited|已邀请
join|加入
joined|已加入
leave|退出
left|已退出
block|屏蔽
unblock|取消屏蔽
report|举报
report spam|举报垃圾信息
spam|垃圾信息
mute for|静音
for 1 hour|1 小时
for 8 hours|8 小时
for 2 days|2 天
forever|永久
customize|自定义
enable|启用
disable|禁用
enabled|已启用
disabled|已禁用
on|开启
off|关闭
yes|是
no|否
ok|好
other|其他
more|更多
less|更少
all|全部
none|无
new|新
old|旧
today|今天
yesterday|昨天
tomorrow|明天
now|现在
just now|刚刚
minute|分钟
minutes|分钟
hour|小时
hours|小时
day|天
days|天
week|周
weeks|周
month|月
months|月
year|年
years|年
second|秒
seconds|秒
ago|前
unread|未读
read|已读
sent|已发送
delivered|已送达
failed|失败
retry|重试
loading|正在加载
please wait|请稍候
error|错误
warning|警告
success|成功
info|信息
help|帮助
support|支持
feedback|反馈
faq|常见问题
version|版本
update|更新
install|安装
download|下载
upload|上传
uploading|正在上传
downloading|正在下载
cache|缓存
media|媒体
auto-download|自动下载
when using mobile data|使用移动数据时
when connected on wi-fi|连接 Wi-Fi 时
when roaming|漫游时
mobile data|移动数据
wi-fi|Wi-Fi
roaming|漫游
data usage|数据用量
storage usage|存储用量
keep media|保留媒体
clear|清除
reset|重置
default|默认
custom|自定义
size|大小
quality|质量
compression|压缩
original|原图
high|高
medium|中
low|低
small|小
large|大
description|描述
title|标题
name|名称
link|链接
invite link|邀请链接
public link|公开链接
username is available|用户名可用
username is already taken|用户名已被占用
invalid|无效
required|必填
optional|选填
change|更改
changed|已更改
copy link|复制链接
share link|分享链接
revoke link|撤销链接
qr code|二维码
scan|扫描
camera|相机
gallery|相册
microphone|麦克风
permission|权限
allow|允许
deny|拒绝
not now|暂时不要
later|稍后
remind me later|稍后提醒我
don't ask again|不再询问
are you sure|确定吗
this action cannot be undone|此操作无法撤销
delete for me|仅为我删除
delete for everyone|为所有人删除
also delete for|同时删除
from|来自
with|与
and|和
or|或
of|的
in|在
at|在
for|为
by|由
your|你的
you|你
me|我
my|我的
we|我们
they|他们
their|他们的
this|此
that|该
these|这些
those|那些
a | 
an | 
the | 
to | 
is|是
are|是
was|是
were|是
be|是
been|是
have|有
has|有
had|有
not|不
don't|不要
cannot|无法
can't|无法
will|将
can|可以
could|可以
should|应该
would|会
may|可能
if|如果
when|当
while|当
after|之后
before|之前
during|期间
without|没有
within|之内
into|到
onto|到
over|超过
under|低于
between|之间
about|关于
up|向上
down|向下
out|出
here|这里
there|那里
please|请
thanks|谢谢
thank you|谢谢
sorry|抱歉
welcome|欢迎
hello|你好
hi|嗨
goodbye|再见
user|用户
users|用户
people|人
person|人
friend|好友
friends|好友
someone|有人
anyone|任何人
everyone|所有人
no one|没有人
self|自己
other device|其他设备
this device|此设备
unknown|未知
empty|空
available|可用
unavailable|不可用
connected|已连接
disconnected|已断开
connecting|正在连接
connection|连接
server|服务器
service|服务
client|客户端
app|应用
application|应用
official|官方
beta|Beta
preview|预览
draft|草稿
scheduled|定时
reminder|提醒
folder|文件夹
folders|文件夹
filter|过滤器
filters|过滤器
tag|标签
tags|标签
topic|话题
topics|话题
thread|话题
reply to|回复
comment|评论
comments|评论
reaction|反应
reactions|反应
like|赞
view|查看
views|次查看
vote|投票
votes|票
option|选项
options|选项
question|问题
answer|回答
correct|正确
incorrect|不正确
anonymous|匿名
public|公开
private|私密
visible|可见
hidden|隐藏
show|显示
hide|隐藏
expand|展开
collapse|收起
details|详情
statistics|统计
boost|助推
boosts|助推
gift|礼物
gifts|礼物
star|星星
stars|星星
premium|Premium
subscription|订阅
subscribe|订阅
unsubscribe|取消订阅
payment|付款
payments|付款
invoice|账单
receipt|收据
balance|余额
wallet|钱包
transaction|交易
price|价格
free|免费
paid|付费
buy|购买
sell|出售
restore|恢复
purchases|购买项目
terms of service|服务条款
privacy policy|隐私政策
accept|接受
decline|拒绝
agree|同意
disagree|不同意
i agree|我同意
learn more|了解更多
read more|阅读更多
see all|查看全部
show more|显示更多
show less|收起
try again|重试
try another|换一个试试
contact support|联系支持
something went wrong|出了点问题
no connection|无网络连接
check your connection|请检查网络连接
no results|没有结果
nothing found|未找到任何内容
no chats|没有聊天
no messages|没有消息
no contacts|没有联系人
start messaging|开始聊天
type a message|输入消息
write a message|写消息
message deleted|消息已删除
chat deleted|聊天已删除
you were removed|你已被移出
you left|你已退出
joined the group|加入了群组
left the group|退出了群组
created the group|创建了群组
changed the group name|修改了群组名称
changed the group photo|修改了群组头像
pinned a message|置顶了一条消息
unpinned a message|取消置顶了一条消息
sent you a message|给你发了一条消息
sent you a photo|给你发了一张图片
sent you a video|给你发了一段视频
sent you a voice message|给你发了一条语音消息
sent you a file|给你发了一个文件
sent you a sticker|给你发了一个贴纸
sent you a gif|给你发了一个 GIF
sent you a location|给你发了一个位置
sent you a contact|给你发了一个联系人
posted a message|发布了一条消息
posted a photo|发布了一张图片
posted a video|发布了一段视频
took a screenshot|截了一张图
encryption|加密
encrypted|已加密
waiting for network|正在等待网络
connecting to proxy|正在连接代理
updating|正在更新
syncing|正在同步
synced|已同步
imported|已导入
import|导入
export|导出
backup|备份
history|历史记录
recent|最近
favorites|收藏
archived|已归档
muted|已静音
pinned|已置顶
unread messages|未读消息
mentions|提及
saved|已保存
editing|正在编辑
select chats|选择聊天
select contacts|选择联系人
new group|新建群组
new channel|新建频道
new chat|新建聊天
add members|添加成员
add people|添加成员
group name|群组名称
channel name|频道名称
description optional|简介（可选）
who can send messages|谁可以发消息
who can add members|谁可以添加成员
slow mode|慢速模式
sign messages|消息署名
discussion|讨论
linked discussion|关联讨论组
join request|加入请求
approve|批准
reject|拒绝
pending|待处理
banned users|被封禁的用户
removed users|被移除的用户
exceptions|例外
send media|发送媒体
send stickers|发送贴纸
embed links|嵌入链接
send polls|发起投票
change info|修改信息
pin messages|置顶消息
add users|添加用户
anonymous admin|匿名管理员
remain anonymous|保持匿名
transfer ownership|转让所有权
delete channel|删除频道
delete group|删除群组
leave channel|退出频道
leave group|退出群组
join channel|加入频道
join group|加入群组
mute notifications|关闭通知
custom notifications|自定义通知
sound|声音
vibrate|振动
popup|弹出通知
priority|优先级
led color|LED 颜色
preview|预览
count|数量
badge|角标
in-app|应用内
lock screen|锁屏
show preview|显示预览
reset all notifications|重置所有通知
when|当
calls|通话
group notifications|群组通知
channel notifications|频道通知
private chats|私密聊天
groups|群组
in app sounds|应用内声音
in-app sounds|应用内声音
in app vibrate|应用内振动
repeat notifications|重复通知
include muted|包括已静音
count unread|统计未读
badge counter|角标计数
draw above|显示在其他应用上层
phone calls|电话
voice calls|语音通话
video calls|视频通话
use less data|节省流量
never|从不
always|始终
automatically|自动
manual|手动
ask|询问
only on wi-fi|仅在 Wi-Fi 下
contacts|联系人
non-contacts|非联系人
everybody|所有人
my contacts|我的联系人
allow calls|允许通话
who can call me|谁可以给我打电话
who can see|谁可以查看
last seen and online|最后上线和在线状态
profile photo|头像
forwarded messages|转发的消息
calls|通话
groups and channels|群组和频道
voice messages|语音消息
bio|简介
date of birth|出生日期
birthday|生日
gifts|礼物
saved music|保存的音乐
phone number|手机号码
add exceptions|添加例外
always share with|始终分享给
never share with|永不分享给
remove exception|移除例外
blocked|已屏蔽
block user|屏蔽用户
unblock user|取消屏蔽
delete contact|删除联系人
edit contact|编辑联系人
share my contact|分享我的联系人
add to contacts|添加到联系人
add to home screen|添加到主屏幕
shortcut|快捷方式
widget|小组件
suggest|建议
suggested|建议
nearby|附近
people nearby|附近的人
make myself visible|让自己可见
make myself invisible|让自己不可见
distance|距离
away|距离
meters|米
kilometers|公里
map|地图
directions|路线
send my current location|发送我的当前位置
stop sharing|停止分享
sharing live location|正在分享实时位置
updated|已更新
accurate to|精确到
search for places|搜索地点
send this location|发送此位置
attach|附加
attachment|附件
caption|说明
album|相册
as file|作为文件
grouped|成组
spoiler|防剧透
paid media|付费媒体
self-destruct|阅后即焚
self destructing|阅后即焚
timer|计时器
off the record|不留记录
screenshot|截图
screenshots|截图
forwarding|转发
saving|保存
restricted|受限制
protected|受保护
content|内容
sensitive|敏感
nsfw|敏感内容
enable filtering|启用过滤
disable filtering|关闭过滤
passcode lock|锁定码
auto-lock|自动锁定
change passcode|更改锁定码
unlock|解锁
fingerprint unlock|指纹解锁
wrong passcode|锁定码错误
too many attempts|尝试次数过多
try again in|请在此时间后重试
recovery email|恢复邮箱
password hint|密码提示
set password|设置密码
change password|更改密码
turn password off|关闭密码
re-enter password|再次输入密码
password do not match|两次输入的密码不一致
create password|创建密码
your password|你的密码
forgot password|忘记密码
reset account|重置账号
delete account|删除账号
deactivate|停用
if away for|若离开
months of inactivity|个月不活跃
confirmation code|验证码
verification code|验证码
enter code|输入验证码
we've sent|我们已发送
sms|短信
call me|给我打电话
resend code|重新发送验证码
wrong code|验证码错误
code expired|验证码已过期
invalid phone number|手机号码无效
phone number banned|手机号码已被封禁
too many requests|请求过于频繁
flood wait|操作过于频繁
internal error|内部错误
network error|网络错误
timeout|超时
unavailable|不可用
maintenance|维护
try again later|请稍后重试
app update|应用更新
a new version is available|有新版本可用
update now|立即更新
what's new|更新说明
remind me|提醒我
skip this version|跳过此版本
you are up to date|已是最新版本
terms|条款
policy|政策
cookies|Cookie
age|年龄
country|国家
region|地区
city|城市
address|地址
choose a country|选择国家
search country|搜索国家
your phone|你的手机
start|开始
get started|开始使用
welcome to|欢迎使用
messaging app|即时通讯应用
fast and secure|快速且安全
cloud|云端
synced across devices|在设备间同步
end to end encrypted|端到端加密
secret chats|私密聊天
disappearing messages|阅后即焚消息
huge groups|超大群组
channels|频道
bots|机器人
bot|机器人
inline|内联
commands|命令
command|命令
menu|菜单
button|按钮
buttons|按钮
keyboard|键盘
reply markup|回复标记
callback|回调
web app|小程序
mini app|小程序
browser|浏览器
open in|打开于
external|外部
copy text|复制文本
translate|翻译
original message|原文
show original|显示原文
select language|选择语言
auto|自动
detected|检测到
from language|源语言
to language|目标语言
speaking|正在说话
transcribing|正在转录
transcript|转录文本
voice to text|语音转文字
playback|播放
speed|速度
pause|暂停
resume|继续
stop|停止
record|录音
hold to record|按住录音
slide to cancel|滑动以取消
release to send|松开发送
release to cancel|松开取消
tap to|点按以
tap|点按
press|按下
hold|按住
swipe|滑动
drag|拖动
pinch|捏合
zoom|缩放
rotate|旋转
crop|裁剪
flip|翻转
draw|绘制
text|文本
font|字体
color|颜色
background|背景
opacity|不透明度
blur|模糊
adjust|调整
filter|滤镜
save to gallery|保存到相册
set as|设为
set as wallpaper|设为壁纸
apply|应用
reset to default|恢复默认
chat background|聊天背景
chat theme|聊天主题
bubble|气泡
color theme|配色
accent|强调色
name color|名字颜色
profile color|个人资料颜色
emoji status|表情状态
set emoji status|设置表情状态
until|直到
clear status|清除状态
collectible|藏品
gift|礼物
send a gift|送礼物
upgrade|升级
convert|转换
transfer|转移
display|展示
wear|佩戴
unique|独特
limited|限量
sold out|已售罄
available|有货
left|剩余
quantity|数量
recipient|接收者
sender|发送者
anonymous gift|匿名礼物
message optional|留言（可选）
pay|支付
confirm|确认
total|合计
fee|手续费
refund|退款
cancelled|已取消
completed|已完成
pending payment|待付款
card|银行卡
apple pay|Apple Pay
google pay|Google Pay
fragment|Fragment
ton|TON
crypto|加密货币
blockchain|区块链
wallet address|钱包地址
connect wallet|连接钱包
disconnect|断开连接
transaction sent|交易已发送
insufficient balance|余额不足
top up|充值
withdraw|提现
history|记录
incoming|收入
outgoing|支出
stars balance|星星余额
buy stars|购买星星
gift stars|赠送星星
earn|获得
spent|已花费
per month|每月
per year|每年
monthly|每月
annual|每年
discount|折扣
trial|试用
restore purchases|恢复购买
manage subscription|管理订阅
already subscribed|已订阅
not subscribed|未订阅
expires|到期
renews|续订
cancel anytime|可随时取消
benefits|权益
faster downloads|更快的下载
more upload|更大的上传
voice to text|语音转文字
no ads|无广告
badge|徽章
animated|动态
custom emoji|定制表情
more folders|更多文件夹
more chats|更多聊天
saved gifs|保存的 GIF
favorite stickers|收藏的贴纸
recent stickers|最近使用的贴纸
trending|热门
installed|已安装
archived stickers|已归档的贴纸
create sticker set|创建贴纸包
add stickers|添加贴纸
remove sticker|移除贴纸
sticker set|贴纸包
emoji set|表情包
search stickers|搜索贴纸
no stickers found|未找到贴纸
send without sound|无声发送
schedule message|定时发送
send later|稍后发送
send now|立即发送
when online|对方上线时
remind me|提醒我
set reminder|设置提醒
edit reminder|编辑提醒
delete reminder|删除提醒
scheduled messages|定时消息
send when online|对方上线时发送
message scheduled|消息已定时
undo|撤销
redo|重做
cut|剪切
bold|粗体
italic|斜体
underline|下划线
strikethrough|删除线
monospace|等宽
spoiler|防剧透
link|链接
quote|引用
code|代码
collapse|收起
expand|展开
formatting|格式
mention|提及
hashtag|话题标签
cashtag|现金标签
command|命令
bot command|机器人命令
start bot|启动机器人
restart|重新开始
stop bot|停止机器人
share bot|分享机器人
add to group|添加到群组
add to channel|添加到频道
bot can't|机器人无法
privacy policy|隐私政策
what can this bot do|这个机器人能做什么
sent by|发送者
via|通过
forwarded from|转发自
channel post|频道帖子
view in chat|在聊天中查看
go to message|转到消息
message not found|未找到消息
message was deleted|消息已删除
this message is unavailable|此消息不可用
loading message|正在加载消息
jump to|跳转到
present|当前
date|日期
time|时间
calendar|日历
pick a date|选择日期
pick a time|选择时间
from date|开始日期
to date|结束日期
apply filter|应用过滤器
reset filter|重置过滤器
sort|排序
newest|最新
oldest|最早
largest|最大
smallest|最小
name a-z|名称 A-Z
kind|类型
all media|全部媒体
photos and videos|图片和视频
files|文件
links|链接
voice|语音
music|音乐
gifs|GIF
common groups|共同群组
shared media|共享媒体
shared links|共享链接
shared files|共享文件
no media|没有媒体
no links|没有链接
no files|没有文件
notifications muted|通知已静音
notifications unmuted|通知已开启
chat archived|聊天已归档
chat unarchived|聊天已取消归档
pinned to top|已置顶
removed from top|已取消置顶
copied to clipboard|已复制到剪贴板
link copied|链接已复制
saved to gallery|已保存到相册
saved to downloads|已保存到下载
download failed|下载失败
upload failed|上传失败
file is too large|文件过大
unsupported format|不支持的格式
open with|打开方式
no app found|未找到可打开的应用
storage permission|存储权限
camera permission|相机权限
microphone permission|麦克风权限
contacts permission|通讯录权限
location permission|位置权限
notification permission|通知权限
photos permission|照片权限
permission required|需要权限
open settings|打开设置
go to settings|前往设置
not allowed|不允许
restricted by|受限制
you can't|你无法
you can|你可以
only admins|仅管理员
all members|所有成员
slow mode is enabled|已开启慢速模式
wait|请等待
seconds left|秒后可发送
you are banned|你已被封禁
you can't send messages|你无法发送消息
you were banned|你已被封禁
appeal|申诉
this channel is private|此频道是私密的
this group is private|此群组是私密的
join to see|加入后查看
request to join|申请加入
request sent|申请已发送
you are a member|你已是成员
already a member|已是成员
invite revoked|邀请已撤销
invite expired|邀请已过期
link is invalid|链接无效
user not found|未找到用户
chat not found|未找到聊天
channel not found|未找到频道
this username is invalid|此用户名无效
this username is occupied|此用户名已被占用
check username|检查用户名
set username|设置用户名
remove username|移除用户名
public username|公开用户名
personal link|个人链接
t.me|t.me
https|https
http|http
www|www
email address|电子邮箱地址
change number|更换号码
new number|新号码
current number|当前号码
number changed|号码已更换
login code|登录验证码
do not give this code to anyone|不要把此验证码告诉任何人
login code is|登录验证码是
we sent a code|我们发送了验证码
sent an sms|已发送短信
calling your phone|正在拨打你的手机
don't accept the call|不要接听来电
dictate the code|语音播报验证码
get code via sms|通过短信获取验证码
get code via call|通过来电获取验证码
get code via fragment|通过 Fragment 获取验证码
sent to the app|已发送到应用
on your other device|在你的其他设备上
check your messages|查看你的消息
enter your name|输入你的名字
add a profile photo|添加头像
first name required|名字（必填）
last name optional|姓氏（选填）
cancel registration|取消注册
registration|注册
almost done|即将完成
sync contacts|同步通讯录
contacts from this device|此设备上的联系人
will be added|将会添加
will not be added|不会添加
turn on|开启
turn off|关闭
added to your account|已添加到你的账号
wrong format|格式错误
number is too short|号码太短
edit number|修改号码
is this the correct number|这个号码正确吗
correct|正确
incorrect number|号码不正确
already logged in|已登录
switch account|切换账号
add account|添加账号
log out of|退出
terminate session|终止会话
terminate all other sessions|终止所有其他会话
active now|当前活动
app version|应用版本
system|系统
ip address|IP 地址
location|位置
unofficial app|非官方应用
this is you|这是你
current session|当前会话
incomplete|未完成
login|登录
logout|退出
security|安全
if this wasn't you|如果这不是你
change password|更改密码
terminate|终止
accept|接受
reject login|拒绝登录
login request|登录请求
new login|新的登录
from a new device|来自新设备
confirm login|确认登录
qr login|二维码登录
scan qr code|扫描二维码
point your camera|将相机对准
log in by qr|通过二维码登录
waiting for confirmation|正在等待确认
login confirmed|登录已确认
login declined|登录已拒绝
passkey|通行密钥
use passkey|使用通行密钥
create passkey|创建通行密钥
passkeys|通行密钥
this device supports|此设备支持
biometrics|生物识别
screen lock|屏幕锁
saved messages info|把消息转发给自己，方便保存
cloud storage|云存储
personal|个人
business|商业
business hours|营业时间
location|位置
address|地址
greeting|问候
away message|离开消息
quick replies|快速回复
shortcut|快捷方式
chat link|聊天链接
start chat|开始聊天
auto reply|自动回复
connected bots|已连接的机器人
pause|暂停
disable|停用
intro|简介
hours|时间
open now|正在营业
closed now|已休息
monday|周一
tuesday|周二
wednesday|周三
thursday|周四
friday|周五
saturday|周六
sunday|周日
weekdays|工作日
weekend|周末
timezone|时区
local time|当地时间
same time|同一时间
quiet hours|免打扰时段
do not disturb|请勿打扰
schedule|时间表
exception|例外
priority chats|优先聊天
allow exceptions|允许例外
sounds|声音
vibration|振动
led|LED
popup notifications|弹出通知
lock screen notifications|锁屏通知
show sender|显示发送者
show content|显示内容
group as|合并为
separate|分开
combined|合并
count|计数
include|包括
exclude|排除
preview text|预览文本
message preview|消息预览
reset notifications|重置通知
notification exceptions|通知例外
custom sound|自定义声音
default sound|默认声音
no sound|无声音
silent|静音
short|短
long|长
only if mentioned|仅在被提及时
disable notifications|关闭通知
enable notifications|开启通知
for this chat|针对此聊天
for all|针对全部
apply to all|应用到全部
existing chats|现有聊天
new chats|新聊天
automatically archive|自动归档
and mute|并静音
from non contacts|来自非联系人
new chats from non-contacts|来自非联系人的新聊天
who aren't in your contacts|不在你的通讯录中
automatically|自动
manual archive|手动归档
keep archived|保持归档
when they reply|当他们回复时
unarchive|取消归档
swipe to|滑动以
archive chats|归档聊天
chat list|聊天列表
compact|紧凑
three lines|三行
two lines|两行
avatar|头像
show avatar|显示头像
message text|消息文本
time|时间
badges|角标
separator|分隔线
animation|动画
reduce motion|减少动态效果
large emoji|大号表情
send with enter|按回车发送
raise to speak|抬起说话
raise to listen|抬起收听
direct share|直接分享
save to gallery|自动保存到相册
stream media|串流媒体
autoplay|自动播放
gifs|GIF
videos|视频
loop|循环
silent playback|静音播放
hd video|高清视频
stream|串流
save incoming|保存收到的
photos|图片
never automatically|从不自动
when battery is low|电量低时
power saving mode|省电模式
animations|动画
autoplay media|自动播放媒体
background data|后台数据
less data|更少数据
calls|通话
peer to peer|点对点
use proxy for calls|通话使用代理
never use peer to peer|从不使用点对点
contacts only|仅联系人
everybody|所有人
system default|系统默认
integrated|集成
in-app browser|应用内浏览器
open links|打开链接
instant view|即时预览
internal|内部
external browser|外部浏览器
ask every time|每次询问
clear cookies|清除 Cookie
clear cache|清除缓存
website|网站
websites|网站
connected websites|已连接的网站
logged in with|登录于
disconnect|断开
revoke|撤销
session|会话
sessions|会话
devices|设备
terminate|终止
incomplete login|未完成的登录
this device|此设备
other devices|其他设备
auto-delete|自动删除
messages auto-delete|消息自动删除
timer|计时器
off|关闭
after|之后
1 day|1 天
1 week|1 周
1 month|1 个月
custom time|自定义时间
set timer|设置计时器
the timer starts|计时从发送后开始
both sides|双方
disappearing|消失
screenshot alert|截图提醒
took a screenshot|截取了屏幕
you took a screenshot|你截取了屏幕
forwarded|已转发
saved|已保存
edited|已编辑
via bot|通过机器人
channel|频道
group|群组
supergroup|超级群组
private group|私密群组
public channel|公开频道
private channel|私密频道
discussion group|讨论群
comments|评论
view comments|查看评论
leave a comment|留下评论
comment|评论
replies|回复
thread|话题
view thread|查看话题
follow|关注
unfollow|取消关注
following|正在关注
followers|关注者
similar channels|相似频道
recommended|推荐
sponsored|赞助
ad|广告
hide ad|隐藏广告
what is this|这是什么
not interested|不感兴趣
report ad|举报广告
remove ad|移除广告
verified|已认证
scam|诈骗
fake|虚假
restricted|受限
premium required|需要 Premium
subscribe to|订阅
to send|才能发送
this feature|此功能
is available|可用
with premium|通过 Premium
get premium|获取 Premium
gift premium|赠送 Premium
months of premium|个月的 Premium
giveaway|赠品活动
participants|参与者
winners|获奖者
prizes|奖品
conditions|条件
ended|已结束
running|进行中
join giveaway|参加赠品活动
you won|你获奖了
better luck|下次好运
results|结果
random|随机
among|之中
boosts channel|助推频道
boost channel|助推频道
boost group|助推群组
your boosts|你的助推
available boosts|可用助推
boost again|再次助推
level|等级
next level|下一级
stories per day|每日动态
custom emoji|定制表情
more|更多
reached|已达到
limit|上限
increase|提高
by boosting|通过助推
current level|当前等级
need|需要
more boosts|更多助推
to reach|以达到
story|动态
stories archive|动态归档
my stories|我的动态
add story|发布动态
view story|查看动态
story expired|动态已过期
close friends|密友
share story|分享动态
reply to story|回复动态
seen by|已查看
viewers|查看者
likes|赞
just you|仅你
selected contacts|选定的联系人
hide from|对谁隐藏
post|发布
camera roll|相机胶卷
recents|最近项目
albums|相册
search photos|搜索照片
allow access|允许访问
to your photos|你的照片
select photos|选择照片
limited access|有限访问
manage|管理
full access|完全访问
video note|视频留言
round video|圆形视频
playback speed|播放速度
pip|画中画
fullscreen|全屏
picture in picture|画中画
speaker|扬声器
earpiece|听筒
bluetooth|蓝牙
headphones|耳机
mute microphone|关闭麦克风
camera off|关闭相机
flip camera|切换相机
end call|结束通话
decline|拒绝
accept call|接听
call again|再次呼叫
busy|忙线
no answer|无人接听
cancelled call|已取消的通话
missed call|未接来电
incoming call|来电
outgoing call|拨出通话
call duration|通话时长
reconnecting|正在重新连接
poor connection|连接不佳
encrypted call|加密通话
key fingerprint|密钥指纹
verify|验证
emoji|表情
compare|比较
if the emoji|如果表情
are the same|相同
the call is secure|通话是安全的
rate call|评价通话
what went wrong|哪里出了问题
send feedback|发送反馈
include logs|包含日志
optional comment|可选备注
submit|提交
thanks for your feedback|感谢你的反馈
debug|调试
copy debug info|复制调试信息
export logs|导出日志
clear logs|清除日志
advanced|高级
experimental|实验性
reset settings|重置设置
reset application|重置应用
delete all data|删除所有数据
this will|这将会
log you out|使你退出登录
and delete|并删除
local data|本地数据
cannot be undone|无法撤销
type to confirm|输入以确认
confirm deletion|确认删除
account deleted|账号已删除
goodbye|再见
come back|欢迎回来
anytime|随时
we will miss you|我们会想你的
deactivate account|停用账号
delete my account|删除我的账号
reason|原因
optional|可选
too many messages|消息太多
privacy concerns|隐私顾虑
found another app|改用其他应用
other reason|其他原因
tell us more|告诉我们更多
send and delete|发送并删除
just delete|仅删除
keep account|保留账号
back to chats|返回聊天
empty chat|空聊天
no messages yet|还没有消息
say hello|打个招呼
wave|挥手
start the conversation|开始对话
you blocked this user|你已屏蔽此用户
unblock to send|取消屏蔽后才能发送
user blocked you|对方已屏蔽你
can't send messages|无法发送消息
to this user|给此用户
privacy settings|隐私设置
don't allow|不允许
messages from|来自的消息
strangers|陌生人
only contacts|仅联系人
charge stars|收取星星
to message you|才能给你发消息
paid messages|付费消息
set price|设置价格
stars per message|每条消息的星星
free for contacts|联系人免费
exceptions|例外
who can message me|谁可以给我发消息
requirements|要求
none|无
stars|星星
removed|已移除
updated successfully|更新成功
saved successfully|保存成功
changes saved|更改已保存
nothing to save|没有可保存的内容
discard changes|放弃更改
keep editing|继续编辑
unsaved changes|未保存的更改
leave without saving|不保存并离开
stay|留下
discard|放弃
apply changes|应用更改
restart required|需要重新启动
restart now|立即重新启动
later|稍后
language changed|语言已更改
interface will update|界面将更新
downloading language|正在下载语言包
language pack|语言包
translation|翻译
fully translated|已全部翻译
partially translated|部分翻译
official translation|官方翻译
help translate|帮助翻译
suggest an edit|建议修改
translation platform|翻译平台
choose your language other|其他
continue on this language|使用此语言继续
"""

_WORD_EXTRA = r"""
message|消息
messages|消息
chat|聊天
chats|聊天
user|用户
users|用户
contact|联系人
contacts|联系人
group|群组
groups|群组
channel|频道
channels|频道
file|文件
files|文件
photo|图片
photos|图片
video|视频
videos|视频
link|链接
links|链接
call|通话
calls|通话
sticker|贴纸
stickers|贴纸
member|成员
members|成员
admin|管理员
admins|管理员
subscriber|订阅者
subscribers|订阅者
notification|通知
notifications|通知
setting|设置
settings|设置
account|账号
password|密码
code|验证码
number|号码
phone|手机
name|名称
text|文本
media|媒体
image|图片
images|图片
document|文件
documents|文件
folder|文件夹
filter|过滤器
sound|声音
theme|主题
color|颜色
size|大小
time|时间
date|日期
info|信息
error|错误
warning|警告
success|成功
help|帮助
search|搜索
share|分享
send|发送
sent|已发送
edit|编辑
edited|已编辑
delete|删除
deleted|已删除
remove|移除
removed|已移除
add|添加
added|已添加
create|创建
created|已创建
open|打开
close|关闭
closed|已关闭
save|保存
saved|已保存
copy|复制
copied|已复制
paste|粘贴
select|选择
selected|已选择
cancel|取消
cancelled|已取消
done|完成
next|下一步
back|返回
skip|跳过
continue|继续
start|开始
stop|停止
retry|重试
update|更新
updated|已更新
download|下载
upload|上传
install|安装
enable|启用
disable|禁用
allow|允许
deny|拒绝
block|屏蔽
blocked|已屏蔽
report|举报
mute|静音
muted|已静音
pin|置顶
pinned|已置顶
archive|归档
archived|已归档
forward|转发
forwarded|已转发
reply|回复
replies|回复
comment|评论
comments|评论
view|查看
views|查看
vote|投票
votes|票
like|赞
join|加入
joined|已加入
leave|退出
left|已退出
invite|邀请
invited|已邀请
request|请求
requested|已请求
accept|接受
accepted|已接受
reject|拒绝
rejected|已拒绝
approve|批准
confirm|确认
confirmed|已确认
change|更改
changed|已更改
set|设置
reset|重置
clear|清除
show|显示
hide|隐藏
hidden|已隐藏
online|在线
offline|离线
available|可用
unavailable|不可用
active|活动
inactive|不活跃
public|公开
private|私密
secret|私密
secure|安全
security|安全
privacy|隐私
encrypted|已加密
official|官方
verified|已认证
scam|诈骗
fake|虚假
restricted|受限
banned|已封禁
premium|Premium
free|免费
paid|付费
new|新
old|旧
all|全部
none|无
more|更多
less|更少
other|其他
custom|自定义
default|默认
automatic|自动
manual|手动
always|始终
never|从不
today|今天
yesterday|昨天
tomorrow|明天
now|现在
soon|即将
later|稍后
again|再次
here|这里
there|那里
yes|是
no|否
ok|好
on|开
off|关
true|是
false|否
unknown|未知
empty|空
loading|加载中
waiting|等待
connecting|连接中
connected|已连接
failed|失败
success|成功
required|必填
optional|可选
invalid|无效
expired|已过期
limit|上限
limited|有限
unlimited|无限制
total|总计
count|数量
list|列表
item|项
items|项
option|选项
options|选项
menu|菜单
button|按钮
screen|屏幕
page|页面
tab|标签页
window|窗口
dialog|对话框
alert|提醒
banner|横幅
title|标题
subtitle|副标题
description|描述
details|详情
summary|摘要
preview|预览
thumbnail|缩略图
icon|图标
image|图片
background|背景
header|标题
footer|页脚
content|内容
body|正文
subject|主题
status|状态
type|类型
format|格式
quality|质量
speed|速度
duration|时长
length|长度
width|宽度
height|高度
position|位置
location|位置
address|地址
country|国家
city|城市
region|地区
language|语言
translation|翻译
version|版本
build|构建
device|设备
devices|设备
session|会话
sessions|会话
network|网络
connection|连接
server|服务器
proxy|代理
storage|存储
cache|缓存
memory|内存
battery|电池
data|数据
usage|用量
history|历史
backup|备份
sync|同步
cloud|云端
local|本地
remote|远程
internal|内部
external|外部
system|系统
app|应用
application|应用
service|服务
support|支持
feedback|反馈
bug|问题
log|日志
debug|调试
test|测试
mode|模式
feature|功能
features|功能
action|操作
actions|操作
event|事件
activity|动态
recent|最近
favorite|收藏
favorites|收藏
popular|热门
trending|热门
recommended|推荐
suggested|建议
nearby|附近
global|全局
personal|个人
business|商业
work|工作
home|家
mobile|手机
desktop|桌面
web|网页
browser|浏览器
camera|相机
gallery|相册
microphone|麦克风
speaker|扬声器
keyboard|键盘
shortcut|快捷方式
widget|小组件
notification|通知
badge|角标
sound|声音
vibration|振动
popup|弹窗
lock|锁定
unlock|解锁
passcode|锁定码
biometric|生物识别
fingerprint|指纹
face|面容
pattern|图案
hint|提示
email|邮箱
sms|短信
call|通话
ring|响铃
missed|未接
incoming|来电
outgoing|拨出
busy|忙线
decline|拒绝
answer|接听
end|结束
hold|保持
mute|静音
unmute|取消静音
video|视频
audio|音频
voice|语音
record|录制
play|播放
pause|暂停
resume|继续
seek|跳转
volume|音量
subtitle|字幕
caption|说明
album|相册
gif|GIF
emoji|表情
reaction|反应
poll|投票
quiz|问答
invoice|账单
payment|付款
gift|礼物
star|星星
stars|星星
boost|助推
level|等级
story|动态
archive|归档
folder|文件夹
topic|话题
thread|话题
comment|评论
mention|提及
hashtag|标签
link|链接
url|URL
qr|二维码
scan|扫描
share|分享
forward|转发
reply|回复
edit|编辑
copy|复制
select|选择
pin|置顶
read|已读
unread|未读
draft|草稿
scheduled|定时
silent|静音
protected|受保护
sensitive|敏感
spoiler|防剧透
anonymous|匿名
visible|可见
distance|距离
map|地图
live|实时
updated|更新于
accurate|精确
meters|米
km|公里
min|分钟
sec|秒
hr|小时
just|刚刚
seen|看到
typing|正在输入
recording|正在录音
uploading|正在上传
downloading|正在下载
processing|正在处理
please|请
wait|等待
it|它
its|它的
where|在哪里
trying|正在尝试
try|尝试
we|我们
we've|我们已
sent|已发送
give|给
this|此
anyone|任何人
do| 
not|不
enter|输入
login|登录
code|验证码
your|你的
you|你
are| 
is| 
was| 
be| 
been| 
have| 
has| 
will|将
can|可以
could|可以
should|应该
would|会
may|可能
if|如果
when|当
that|该
this|此
these|这些
those|那些
there|那里
here|这里
just|就
only|仅
also|也
still|仍
already|已经
yet|还
very|非常
really|确实
now|现在
then|然后
than|比
so|所以
because|因为
but|但
however|然而
about|关于
into|到
over|超过
under|在...下
between|之间
without|没有
within|在...内
during|在...期间
after|在...之后
before|在...之前
while|当
until|直到
since|自从
from|从
with|和
for|为
and|和
or|或
of|的
in|在
on|在
at|在
by|由
to| 
a| 
an| 
the| 
as|作为
via|通过
per|每
each|每个
every|每个
all|所有
some|一些
any|任何
no|无
not|不
don't|不要
doesn't|不
didn't|没有
can't|无法
cannot|无法
won't|不会
isn't|不是
aren't|不是
wasn't|不是
weren't|不是
i'm|我
you're|你
we're|我们
they're|他们
it's|它
that's|那是
there's|有
here's|这里是
i've|我已
you've|你已
let's|让我们
i|我
my|我的
me|我
our|我们的
us|我们
their|他们的
them|他们
they|他们
he|他
she|她
his|他的
her|她的
him|他
who|谁
what|什么
which|哪个
how|如何
why|为什么
when|何时
where|哪里
check|检查
enter|输入
choose|选择
pick|选取
tap|点按
press|按
hold|按住
slide|滑动
swipe|轻扫
drag|拖动
release|松开
click|点击
open|打开
close|关闭
exit|退出
quit|退出
finish|完成
complete|完成
success|成功
failed|失败
wrong|错误
correct|正确
invalid|无效
expired|过期
banned|封禁
restricted|受限
allowed|已允许
denied|已拒绝
granted|已授予
missing|缺失
found|找到
results|结果
empty|空
full|已满
too|太
many|许多
few|很少
some|一些
any|任何
every|每个
each|每个
own|自己的
self|自己
another|另一个
same|相同
different|不同
similar|相似
following|以下
above|以上
below|以下
between|之间
without|没有
with|带有
from|从
into|进入
about|关于
via|通过
per|每
only|仅
also|也
still|仍然
already|已经
yet|尚未
again|再次
once|一次
twice|两次
first|第一
last|最后
next|下一个
previous|上一个
current|当前
recently|最近
soon|很快
later|稍后
early|早
late|晚
fast|快
slow|慢
high|高
low|低
best|最佳
better|更好
worse|更差
good|好
bad|差
great|很好
important|重要
optional|可选
required|必需
available|可用
enabled|已启用
disabled|已禁用
active|活动的
inactive|未活动
public|公开的
private|私密的
secure|安全的
unsafe|不安全
temporary|临时
permanent|永久
automatic|自动的
manual|手动的
custom|自定义
built-in|内置
remote|远程
local|本地
global|全局
personal|个人
shared|共享
common|共同
direct|直接
indirect|间接
incoming|传入
outgoing|传出
internal|内部
external|外部
online|在线
offline|离线
away|离开
busy|忙碌
invisible|隐身
visible|可见
hidden|隐藏
shown|已显示
collapsed|已收起
expanded|已展开
selected|已选
unselected|未选
checked|已勾选
unchecked|未勾选
pinned|已置顶
unpinned|未置顶
muted|已静音
unmuted|未静音
archived|已归档
unarchived|未归档
read|已读
unread|未读
sent|已发送
received|已收到
delivered|已送达
seen|已读
edited|已编辑
deleted|已删除
removed|已移除
added|已添加
created|已创建
updated|已更新
changed|已更改
saved|已保存
unsaved|未保存
copied|已复制
moved|已移动
renamed|已重命名
uploaded|已上传
downloaded|已下载
installed|已安装
uninstalled|已卸载
connected|已连接
disconnected|已断开
joined|已加入
left|已离开
invited|已邀请
kicked|已踢出
banned|已封禁
unbanned|已解封
blocked|已屏蔽
unblocked|已取消屏蔽
reported|已举报
ignored|已忽略
accepted|已接受
declined|已拒绝
approved|已批准
rejected|已拒绝
cancelled|已取消
completed|已完成
pending|待处理
failed|已失败
succeeded|已成功
expired|已过期
revoked|已撤销
invalid|无效
valid|有效
required|必填
missing|缺失
found|已找到
matched|已匹配
applied|已应用
reset|已重置
cleared|已清除
restored|已恢复
imported|已导入
exported|已导出
synced|已同步
backed|已备份
scheduled|已定时
postponed|已推迟
reminded|已提醒
notified|已通知
mentioned|已提及
quoted|已引用
forwarded|已转发
replied|已回复
reacted|已回应
voted|已投票
liked|已赞
viewed|已查看
played|已播放
paused|已暂停
stopped|已停止
started|已开始
finished|已结束
opened|已打开
closed|已关闭
locked|已锁定
unlocked|已解锁
enabled|已开启
disabled|已关闭
allowed|已允许
denied|已拒绝
granted|已授权
requested|已请求
confirmed|已确认
verified|已验证
unverified|未验证
authenticated|已认证
unauthorized|未授权
logged|已登录
registered|已注册
unregistered|未注册
subscribed|已订阅
unsubscribed|已取消订阅
boosted|已助推
gifted|已赠送
purchased|已购买
refunded|已退款
paid|已支付
unpaid|未支付
free|免费
premium|Premium
official|官方
beta|Beta
alpha|Alpha
experimental|实验
deprecated|已弃用
unsupported|不支持
supported|支持
compatible|兼容
incompatible|不兼容
recommended|推荐
optional|可选
mandatory|强制
temporary|临时
permanent|永久
anonymous|匿名
public|公开
private|私密
secret|私密
end-to-end|端到端
e2e|端到端
p2p|点对点
voip|网络通话
hd|高清
sd|标清
gif|GIF
png|PNG
jpg|JPG
mp4|MP4
pdf|PDF
zip|ZIP
url|URL
id|ID
ip|IP
qr|二维码
ok|好
yes|是
no|否
want|想要
access|访问
use|使用
uses|使用
used|已使用
using|正在使用
see|查看
able|能够
community|社区
get|获取
got|已获得
getting|正在获取
one|一个
receive|接收
receiving|正在接收
sending|正在发送
phrase|短语
keep|保留
sharing|正在分享
offer|提供
top|顶部
make|让
makes|让
made|已
additional|其他
must|必须
bid|出价
switch|切换
pack|资源包
packs|资源包
posted|发布了
scored|得了
score|分数
longer|更长时间
move|移动
currently|当前
passport|护照
style|样式
program|项目
go|去
going|正在
let|让
cellular|蜂窝
continuously|持续
provided|提供的
including|包括
remaining|剩余
technical|技术
identify|找出
problem|问题
space|空间
importing|导入
unlike|与其他不同
target|投放
ads|广告
based|基于
need|需要
needs|需要
needed|需要
correct|正确
short|太短
option|选项
options|选项
details|详细信息
detail|详细信息
so|因此
works|有效
work|工作
working|正在工作
doesn|不
doesnt|不
won|不会
wont|不会
isn|不是
wasn|不是
couldn|无法
shouldn|不应该
people|用户
person|人
friends|好友
friend|好友
someone|有人
anyone|任何人
everyone|所有人
nobody|没有人
something|某些内容
anything|任何内容
everything|所有内容
nothing|没有任何内容
somewhere|某处
anywhere|任何地方
enough|足够
almost|几乎
quite|相当
rather|相当
really|确实
actually|实际上
probably|可能
maybe|也许
perhaps|也许
sure|确定
available|可用
following|以下
below|下方
above|上方
inside|内部
outside|外部
together|一起
alone|单独
instead|而是
otherwise|否则
however|但是
therefore|因此
example|示例
information|信息
description|描述
successfully|成功
automatically|自动
manually|手动
directly|直接
completely|完全
recently|最近
usually|通常
always|始终
never|从不
sometimes|有时
often|经常
once|一次
twice|两次
several|几个
multiple|多个
single|单个
whole|整个
part|部分
parts|部分
rest|其余
kind|类型
sort|种类
way|方式
ways|方式
thing|内容
things|内容
place|位置
places|位置
time|时间
times|次
day|天
days|天
week|周
weeks|周
month|月
months|月
year|年
years|年
hour|小时
hours|小时
minute|分钟
minutes|分钟
second|秒
seconds|秒
moment|片刻
while|一会儿
until|直到
since|自从
during|期间
before|之前
after|之后
soon|很快
later|稍后
early|提前
late|延迟
fast|快速
slow|慢速
quick|快速
quickly|快速
slowly|缓慢
high|高
low|低
long|长
full|完整
empty|空
small|小
large|大
big|大
little|少
own|自己的
same|相同
different|不同
important|重要
possible|可能
impossible|不可能
necessary|必要
simple|简单
easy|容易
hard|困难
right|正确
wrong|错误
true|真
false|假
real|真实
main|主要
general|常规
special|特殊
normal|正常
common|常见
specific|特定
local|本地
global|全球
public|公开
private|私密
safe|安全
secure|安全
sorry|抱歉
thanks|谢谢
hello|你好
welcome|欢迎
please|请
note|注意
warning|警告
error|错误
errors|错误
failed|失败
failure|失败
success|成功
result|结果
results|结果
list|列表
item|项目
items|项目
section|部分
page|页面
screen|屏幕
window|窗口
button|按钮
field|字段
value|值
values|值
key|键
keys|键
type|类型
types|类型
format|格式
size|大小
limit|限制
limits|限制
level|等级
amount|数量
number|数字
count|计数
total|总计
percent|百分比
version|版本
update|更新
updates|更新
feature|功能
features|功能
support|支持
help|帮助
question|问题
questions|问题
answer|答案
answers|答案
issue|问题
issues|问题
request|请求
requests|请求
response|响应
service|服务
services|服务
server|服务器
client|客户端
device|设备
devices|设备
phone|手机
number|号码
email|邮箱
address|地址
name|名称
names|名称
title|标题
text|文本
content|内容
message|消息
messages|消息
chat|聊天
chats|聊天
user|用户
users|用户
group|群组
groups|群组
channel|频道
channels|频道
contact|联系人
contacts|联系人
member|成员
members|成员
admin|管理员
file|文件
files|文件
photo|照片
photos|照片
video|视频
videos|视频
image|图片
images|图片
link|链接
links|链接
call|通话
voice|语音
audio|音频
music|音乐
sticker|贴纸
media|媒体
notification|通知
notifications|通知
setting|设置
settings|设置
account|账号
accounts|账号
password|密码
code|验证码
codes|验证码
session|会话
sessions|会话
network|网络
connection|连接
data|数据
storage|存储
cache|缓存
history|历史记录
backup|备份
sync|同步
language|语言
theme|主题
color|颜色
wallpaper|壁纸
profile|个人资料
status|状态
online|在线
offline|离线
privacy|隐私
security|安全
permission|权限
permissions|权限
exception|例外
block|屏蔽
report|举报
spam|垃圾信息
mute|静音
pin|置顶
archive|归档
forward|转发
reply|回复
edit|编辑
delete|删除
remove|移除
add|添加
create|创建
open|打开
close|关闭
save|保存
copy|复制
paste|粘贴
select|选择
cancel|取消
done|完成
continue|继续
skip|跳过
back|返回
next|下一步
start|开始
stop|停止
retry|重试
send|发送
sent|已发送
share|分享
search|搜索
show|显示
hide|隐藏
enable|启用
disable|禁用
allow|允许
deny|拒绝
confirm|确认
change|更改
reset|重置
clear|清除
apply|应用
install|安装
download|下载
upload|上传
import|导入
export|导出
join|加入
leave|退出
invite|邀请
accept|接受
decline|拒绝
approve|批准
reject|拒绝
view|查看
play|播放
pause|暂停
record|录制
scan|扫描
tap|点按
press|按
hold|按住
slide|滑动
release|松开
choose|选择
enter|输入
check|检查
turn|开启
switch|切换
set|设置
get|获取
make|进行
take|进行
give|提供
find|查找
found|找到
try|尝试
wait|等待
loading|正在加载
processing|正在处理
connecting|正在连接
updating|正在更新
syncing|正在同步
sending|正在发送
receiving|正在接收
revenue|收入
restrict|限制
restricted|已限制
cover|封面
auction|拍卖
sale|出售
checklist|清单
maximum|最大
displayed|显示的
ban|封禁
banned|已封禁
affiliate|联盟
affiliates|联盟
order|顺序
existing|现有
masks|遮罩
mask|遮罩
funds|资金
verification|验证
appear|显示
appears|显示
transferred|已转移
transfer|转移
notify|通知
notified|已通知
connect|连接
connected|已连接
published|已发布
publish|发布
publishing|正在发布
proceed|继续
proceeds|收益
administrators|管理员
administrator|管理员
acquired|已获得
such|此类
word|词
words|词
mini|迷你
commission|佣金
prize|奖品
prizes|奖品
entire|整个
recovery|恢复
collection|收藏
collections|收藏
converted|已转换
convert|转换
future|将来
least|至少
third|第三
party|方
purchase|购买
purchased|已购买
contains|包含
contain|包含
characters|字符
character|字符
subscribing|订阅
listed|已列出
included|已包含
auto|自动
lost|丢失
lose|失去
tasks|任务
task|任务
symbol|符号
symbols|符号
double|双倍
randomly|随机
exclusive|专属
older|更早
process|处理
become|成为
crafting|制作
craft|制作
crafted|已制作
adding|正在添加
approval|批准
shipping|运送
review|查看
provide|提供
round|圆形
replace|替换
invitations|邀请
invitation|邀请
stealth|隐身
period|期间
upgraded|已升级
upgrade|升级
broadcast|广播
bought|已购买
speak|说话
viewing|正在查看
mail|邮件
require|需要
requires|需要
rating|评分
charge|收取
occurred|发生
watching|正在观看
activate|激活
money|金额
anyway|无论如何
turned|已切换
platform|平台
related|相关
reorder|重新排序
winner|获奖者
winners|获奖者
opening|正在打开
fact|事实上
model|模型
models|模型
mark|标记
marked|已标记
minimum|最小
creating|正在创建
posting|正在发布
gifting|赠送
rewards|奖励
reward|奖励
backdrop|背景
entered|已输入
remain|保持
faster|更快
stored|已存储
reduce|减少
side|侧
developer|开发者
know|知道
author|作者
picture|图片
unable|无法
suggestions|建议
suggestion|建议
highlight|高亮
taken|已使用
worth|价值
restrictions|限制
joining|正在加入
ownership|所有权
peer|对等
referral|推荐
checking|正在检查
reserved|已保留
participating|正在参与
identity|身份
expire|过期
expires|过期
except|除了
abort|中止
logo|标志
spending|花费
daily|每日
calling|正在呼叫
night|夜间
due|到期
costs|费用
cost|费用
understood|知道了
store|商店
kept|已保留
copying|正在复制
sold|已售出
messaging|发消息
write|编写
yourself|你自己
disk|磁盘
streaming|串流
english|英语
remember|记住
being|正在
eligible|符合条件
withdrawal|提现
average|平均
protect|保护
certain|某些
couldn't|无法
you'll|你将
you're|你
they're|他们
it's|这是
that's|那是
there's|有
don't|不要
doesn't|不
didn't|没有
can't|无法
cannot|无法
won't|不会
isn't|不是
aren't|不是
wasn't|不是
i'm|我
i've|我已
we've|我们已
let's|让我们
re|重新
up|向上
game|游戏
games|游戏
even|甚至
most|最多
peer|端
peers|端
database|数据库
cached|已缓存
cache|缓存
certified|已认证
motion|动画
sim|SIM
recorded|已录制
e-mail|电子邮箱
email|电子邮箱
third-party|第三方
top-up|充值
slowmode|慢速模式
couldn't|无法
wouldn't|不会
shouldn't|不应该
reccurrent|定期
recurrent|定期
recurring|定期
"""


def _load_map(raw: str) -> dict[str, str]:
    out = {}
    for line in raw.splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "|" not in line:
            continue
        src, dst = line.split("|", 1)
        out[src.strip().lower()] = dst.strip()
    return out


_PHRASES = _load_map(_PHRASES_RAW)
_WORDS = _load_map(_WORD_EXTRA)
for key, val in list(_PHRASES.items()):
    if " " not in key:
        _WORDS.setdefault(key, val)
_PHRASE_BY_LEN: dict[int, dict[tuple[str, ...], str]] = {}
for src, dst in _PHRASES.items():
    parts = tuple(src.split())
    if len(parts) < 2:
        continue
    _PHRASE_BY_LEN.setdefault(len(parts), {})[parts] = dst
_MAX_PHRASE = max(_PHRASE_BY_LEN) if _PHRASE_BY_LEN else 1

# Date/number format tokens that must stay literal, plus product names.
_FORMAT_TOKENS = {
    "mmm", "mmmm", "eee", "eeee", "yyyy", "yyy", "yy", "hh", "mm", "dd", "ss",
    "mmm", "boc", "base64", "rtmp", "cvv", "usd", "fhd", "qhd", "2k", "4k",
}

_ALLOW = {
    "tellomi", "gram", "grams", "instagram", "yyyy", "mmm", "mm", "hh", "dd", "ss",
    "ok", "gif", "gifs", "sms", "qr", "pin", "wi-fi", "wifi", "bluetooth",
    "url", "id", "api", "nft", "premium", "google", "apple", "fragment", "youtube",
    "instagram", "twitter", "facebook", "whatsapp", "ios", "android", "macos", "mac",
    "windows", "linux", "http", "https", "pdf", "mp3", "mp4", "zip", "ton", "usdt",
    "btc", "vpn", "dns", "tcp", "udp", "stun", "turn", "webrtc", "json", "html",
    "sdk", "uid", "dc", "mtproto", "ssl", "tls", "hd", "kb", "mb", "gb", "am", "pm",
    "vip", "cdn", "usb", "nfc", "gps", "sim", "2fa", "ai", "siri", "spotify", "ip",
    "png", "jpg", "jpeg", "webp", "apk", "iphone", "ipad", "safari", "chrome",
    "paypal", "visa", "github", "bot", "bots", "web", "pc", "tv", "4k", "hdr",
    "opus", "ogg", "wav", "heic", "emoji", "emojis", "beta", "alpha",
    "dropbox", "icloud", "yandex", "waze", "apple",
    "chromecast", "airplay", "youtube", "duckduckgo", "bing", "yahoo",
    "wearos", "toncoin", "rnnoise", "latex", "roboto", "merriweather",
    "courier", "cocoon", "lucio", "nox", "dino", "fragment", "socks5",
    "mtproto", "hasselhoff", "suntory", "covid", "psa", "cvv", "boc",
    "base64", "rtmp", "usd", "fhd", "qhd", "mono", "serif",
    "telegramtips", "brave", "google",
    # Onboarding personas and protocol names that stay in Latin inside sentences.
    "reinhardt", "bob", "alice", "jennifer", "alicia", "torreaux", "roberto",
    "veronica", "james", "emma", "gabriella", "beatrice", "jessica", "jennie",
    "harris", "newman", "monika", "paul", "kate", "nick", "adler", "toberg",
    "david", "eva", "summer", "mike", "bright", "lucio", "walt", "botfather",
    "rove", "socks5", "camera2", "webview", "stateinit", "mnc", "led", "faq",
    "passport", "icloud", "hasselhoff", "spotlight", "mtproxy",
    "dapp", "p2plp", "fps",
    "harry", "helene", "dwayne", "roxanne",
    "id", "qr", "pin", "sim", "gif", "sms", "bot", "bots", "app",
    "web", "pc", "tv", "hd", "ip", "url", "api", "nft", "json",
    "html", "sdk", "ssl", "tls", "http", "https", "www", "ios",
    "android", "macos", "windows", "linux", "iphone", "ipad",
    "safari", "chrome", "paypal", "visa", "github", "spotify",
    "siri", "instagram", "twitter", "facebook", "whatsapp",
    "premium", "tellomi", "gram", "grams", "ok", "beta", "alpha",
}

# Entire string is a product sound name (iOS), not prose.
_SOUND_NAMES = {
    "aurora", "bamboo", "chord", "circles", "input", "popcorn", "pulse", "synth",
    "rebound", "antic", "cheers", "droplet", "handoff", "milestone", "passage",
    "portal", "rattle", "tri-tone", "tremolo", "bell", "calypso", "chime", "glass",
    "telegraph", "socks5", "led",
}

_UNKNOWN: Counter[str] = Counter()


def unknown_summary(limit: int = 40) -> list[tuple[str, int]]:
    return _UNKNOWN.most_common(limit)


_URL = re.compile(r"https?://[^\s<>()]+|t\.me/[^\s<>()]+|www\.[^\s<>()]+", re.I)


def brand(text: str) -> str:
    """Replace the app name. Leave telegram.org, t.me, and @telegram alone."""
    slots: list[str] = []

    def hold(match: re.Match[str]) -> str:
        slots.append(match.group(0))
        return f"\u0000{len(slots) - 1}\u0000"

    shielded = _URL.sub(hold, text)

    def repl(match: re.Match[str]) -> str:
        word = match.group(0)
        if word.isupper():
            return "TELLOMI"
        if word[:1].islower():
            return "Tellomi"
        return "Tellomi"

    shielded = re.sub(r"(?<![@./\w])[Tt]elegram(?!\.org\b)(?!\w)|(?<![@./\w])TELEGRAM(?!\w)", repl, shielded)
    for i, url in enumerate(slots):
        shielded = shielded.replace(f"\u0000{i}\u0000", url)
    return shielded


def _lookup_word(word: str) -> str | None:
    key = word.lower().strip(".,;:!?()[]\"'")
    if not key:
        return word
    if key in _WORDS:
        return _WORDS[key]
    if key.endswith("'s") and key[:-2] in _WORDS:
        return _WORDS[key[:-2]] + "的"
    if key.endswith("ies") and key[:-3] + "y" in _WORDS:
        return _WORDS[key[:-3] + "y"]
    if key.endswith("es") and key[:-2] in _WORDS:
        return _WORDS[key[:-2]]
    if key.endswith("s") and not key.endswith("ss") and key[:-1] in _WORDS:
        return _WORDS[key[:-1]]
    if key in _ALLOW or key.startswith("tellomi"):
        return word if word.lower() != "tellomi" else "Tellomi"
    if re.fullmatch(r"\d+([.,]\d+)?", key):
        return word
    return None


def _translate_simplified(text: str) -> tuple[str, list[str]]:
    if text.strip() == "":
        return text, []
    pieces: list[tuple[str, str]] = []
    pos = 0
    for match in _TOKEN.finditer(text):
        if match.start() > pos:
            pieces.append(("text", text[pos : match.start()]))
        pieces.append(("tok", match.group(0)))
        pos = match.end()
    if pos < len(text):
        pieces.append(("text", text[pos:]))

    missing: list[str] = []
    out: list[str] = []
    for kind, chunk in pieces:
        if kind == "tok":
            out.append(chunk)
            continue
        out.append(_translate_chunk(chunk, missing))
    translated = "".join(out)
    translated = re.sub(r"[ \t]{2,}", " ", translated)
    translated = re.sub(r"(?<=[\u4e00-\u9fff])\s+(?=[\u4e00-\u9fff])", "", translated)
    translated = re.sub(r"(?<=[\u4e00-\u9fff])\s+(?=[，。！？、])", "", translated)
    translated = re.sub(r" +([，。！？、])", r"\1", translated)
    translated = re.sub(r"。+\.", "。", translated)
    translated = re.sub(r"？+\?", "？", translated)
    translated = re.sub(r"！+!", "！", translated)
    if re.search(r"[\u4e00-\u9fff]", translated):
        translated = re.sub(r"\?(?=(\s|$))", "？", translated)
        translated = re.sub(r"!(?=(\s|$))", "！", translated)
    return translated.strip() if translated.strip() else translated, missing


def _translate_chunk(chunk: str, missing: list[str]) -> str:
    tokens = re.findall(r"[A-Za-z0-9]+(?:[-'][A-Za-z0-9]+)*|[^\sA-Za-z0-9]+|\s+", chunk)
    word_at = [i for i, tok in enumerate(tokens) if re.match(r"[A-Za-z0-9]", tok)]
    # Map word-list index -> (phrase length in words, translation), only on the first word.
    phrase_at: dict[int, tuple[int, str]] = {}
    i = 0
    while i < len(word_at):
        matched = False
        remain = len(word_at) - i
        for length in range(min(_MAX_PHRASE, remain), 1, -1):
            seq = tuple(tokens[word_at[i + n]].lower().strip(".,;:!?()[]\"'") for n in range(length))
            hit = _PHRASE_BY_LEN.get(length, {}).get(seq)
            if hit is not None:
                phrase_at[i] = (length, hit)
                i += length
                matched = True
                break
        if not matched:
            i += 1

    out: list[str] = []
    word_i = 0
    skip_tokens_until = -1
    for idx, tok in enumerate(tokens):
        if idx < skip_tokens_until:
            continue
        is_word = bool(re.match(r"[A-Za-z0-9]", tok))
        if is_word and word_i in phrase_at:
            length, dst = phrase_at[word_i]
            out.append(dst)
            skip_tokens_until = word_at[word_i + length - 1] + 1
            word_i += length
            continue
        if is_word:
            looked = _lookup_word(tok)
            if looked is None:
                bare = tok.lower().strip(".,;:!?()[]\"'")
                if bare and not re.fullmatch(r"\d+", bare):
                    missing.append(bare)
                    _UNKNOWN[bare] += 1
                out.append(tok)
            else:
                punct = re.search(r"[.,;:!?]+$", tok)
                out.append(looked + (punct.group(0) if punct else ""))
            word_i += 1
            continue
        out.append(tok)
    return "".join(out)


def _exact_translations() -> dict[str, str]:
    found: dict[str, str] = {}
    root = Path(__file__).resolve().parent / "exact"
    if not root.is_dir():
        return found
    for path in sorted(root.glob("*.py")):
        ns: dict = {}
        exec(path.read_text(encoding="utf-8"), ns)  # noqa: S102 - local translation tables
        found.update(ns.get("PAIRS", {}))
    # JSON tables override older glossary pairs when a string was rewritten.
    for path in sorted(root.glob("*.json")):
        data = json.loads(path.read_text(encoding="utf-8"))
        if isinstance(data, dict):
            found.update(data)
        elif isinstance(data, list):
            for item in data:
                found[item["en"]] = item["zh"]
    return found


_EXACT = _exact_translations()


def leftover_words(text: str) -> list[str]:
    """English words that are not placeholders, brands, or proper nouns."""
    cleaned = text.replace("’", "'").replace("‘", "'").replace("ʼ", "'")
    cleaned = re.sub(
        r"https?://\S+|[\w.-]*telegram\.org\S*|t\.me/\S+|[\w.-]+\.(?:com|org|net)\S*",
        " ",
        cleaned,
        flags=re.I,
    )
    cleaned = _TOKEN.sub(" ", cleaned)
    cleaned = re.sub(r"@[A-Za-z0-9_]+", " ", cleaned)
    cleaned = re.sub(r":[A-Za-z0-9_]+:", " ", cleaned)
    cleaned = re.sub(
        r"\b[\w.-]+\.(?:ppd|pdf|docx?|xlsx?|pptx?|zip|mp3|mp4|jpe?g|png|gif|txt|key|mov|apk)\b",
        " ",
        cleaned,
        flags=re.I,
    )
    # Markdown link targets such as [text](username) are client identifiers.
    cleaned = re.sub(r"\]\([A-Za-z0-9_]+\)", " ", cleaned)
    stripped = cleaned.strip()
    # FAQ anchors (q-i-have-a-new-phone-number) and names like Tri-tone.
    if re.fullmatch(r"[A-Za-z0-9]+(?:-[A-Za-z0-9]+)+", stripped):
        return []
    if stripped.lower() in _SOUND_NAMES or stripped.lower() in _ALLOW:
        return []
    cleaned = re.sub(r"(?i)\b(?:app store|google play|face id|apple watch|touch id)\b", " ", cleaned)

    def strip_known(match: re.Match[str]) -> str:
        key = match.group(0).lower().strip("'")
        if key in _ALLOW or key in _FORMAT_TOKENS:
            return " "
        return match.group(0)

    cleaned = re.sub(r"[A-Za-z][A-Za-z0-9']*", strip_known, cleaned)
    bad = []
    for word in re.findall(r"[A-Za-z][A-Za-z']{2,}", cleaned):
        key = word.lower().strip("'")
        if key.endswith("'s") and key[:-2] in _ALLOW:
            continue
        if key in _ALLOW or key in _FORMAT_TOKENS:
            continue
        bad.append(key)
    return bad


def _pattern_natural(text: str) -> str | None:
    """Natural Chinese for dates, durations, and a few fixed frames."""
    months = {
        "january": "1月", "february": "2月", "march": "3月", "april": "4月",
        "june": "6月", "july": "7月", "august": "8月", "september": "9月",
        "october": "10月", "november": "11月", "december": "12月",
        "jan": "1月", "feb": "2月", "mar": "3月", "apr": "4月",
        "jun": "6月", "jul": "7月", "aug": "8月", "sep": "9月",
        "oct": "10月", "nov": "11月", "dec": "12月",
    }
    weeks = {
        "mon": "周一", "tue": "周二", "wed": "周三", "thu": "周四",
        "fri": "周五", "sat": "周六", "sun": "周日",
    }
    units = {"s": "秒", "m": "分钟", "h": "小时", "d": "天", "w": "周", "mo": "个月", "y": "年"}
    stripped = text.strip()
    low = stripped.lower()
    if low in months:
        return months[low]
    if low in weeks:
        return weeks[low]
    letters = {"M": "一", "T": "二", "W": "三", "F": "五", "S": "六", "L": "大"}
    if stripped in letters:
        return letters[stripped]
    unit_letters = {"s": "秒", "m": "分钟", "h": "小时", "d": "天", "w": "周", "y": "年", "mi": "英里"}
    if stripped in unit_letters:
        return unit_letters[stripped]
    matched = re.fullmatch(r"(January|February|March|April|June|July|August|September|October|November|December) (%@)", stripped)
    if matched:
        return months[matched.group(1).lower()] + " " + matched.group(2)
    matched = re.fullmatch(
        r"(Jan|Feb|Mar|Apr|Jun|Jul|Aug|Sep|Oct|Nov|Dec) %1\$@, %2\$@ at %3\$@",
        stripped,
    )
    if matched:
        return f"{months[matched.group(1).lower()]}%1$@日 %2$@ %3$@"
    matched = re.fullmatch(
        r"(January|February|March|April|June|July|August|September|October|November|December) %1\$@, %2\$@",
        stripped,
    )
    if matched:
        return f"{months[matched.group(1).lower()]}%1$@日 %2$@"
    matched = re.fullmatch(r"(%@|%\d+\$d|%d)(s|m|h|d|w|mo|y)", stripped)
    if matched:
        return matched.group(1) + units[matched.group(2)]
    matched = re.fullmatch(r"(\d+)(s|m|h|d|w)", stripped)
    if matched:
        return matched.group(1) + units[matched.group(2)]
    matched = re.fullmatch(r"(%@) (h|m)", stripped)
    if matched:
        return matched.group(1) + ("小时" if matched.group(2) == "h" else "米")
    return None


def translate(text: str, lang: str) -> tuple[str, list[str]]:
    if lang == "en":
        return text, []
    exact = _EXACT.get(text)
    patterned = None if exact is not None else _pattern_natural(text)
    if exact is not None and _TOKEN.findall(text) == _TOKEN.findall(exact):
        simplified, missing = exact, leftover_words(exact)
    elif patterned is not None and _TOKEN.findall(text) == _TOKEN.findall(patterned):
        simplified, missing = patterned, leftover_words(patterned)
    else:
        simplified, missing = _translate_simplified(text)
        missing = leftover_words(simplified)
    if lang == "zh-hant":
        # Keep placeholders stable across OpenCC.
        slots = []

        def hold(match: re.Match[str]) -> str:
            slots.append(match.group(0))
            return f"⟦{len(slots) - 1}⟧"

        shielded = _TOKEN.sub(hold, simplified)
        converted = _OPENCC.convert(shielded)
        for i, token in enumerate(slots):
            converted = converted.replace(f"⟦{i}⟧", token)
        return converted, missing
    return simplified, missing
