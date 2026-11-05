-- D-56 / appendix D last row. Appendix C names that no official account owns.
--
-- account.checkUsername treats any username row as taken, including
-- peer_type 0. TELLOMI_USERNAME_POLICY defaults to off, so these rows are
-- what stops a normal user from registering the exact name. Case follows
-- utf8mb4_unicode_ci: WeChat and WECHAT are this row.
--
-- peer_type 0 and peer_id 0 are an empty placeholder. There is no user
-- and no channel. user.resolveUsername returns USERNAME_NOT_OCCUPIED for
-- that peer type, the same as the unassigned bot names in
-- migrate-20261105-official-accounts.sql. contacts.resolveUsername cannot
-- open a chat. account.checkUsername still returns USERNAME_OCCUPIED.
--
-- The list is pkg/usernamepolicy.AppendixCPlaceholders, the overlay in
-- spec.go whose source is requirements-v2/appendix-c, minus names appendix
-- D already assigned. qq, jd, x, and xai are shorter than the public
-- username minimum (4) and are not inserted. Underscore affixes
-- (wechat_01) and the 470-term lexicon are not rows; those still need
-- TELLOMI_USERNAME_POLICY=1.
--
-- username.id is AUTO_INCREMENT. A second run changes nothing. If any of
-- these names already belongs to another peer, the script stops and does
-- not take it (D-26). allowlist.toml is in tellomi/tellomi, not this repo.

SET @bad = (SELECT username FROM username WHERE username IN (
'alibaba',
'alipay',
'aliyun',
'amazon',
'anthropic',
'apple',
'appstore',
'baidu',
'bilibili',
'botfather',
'bytedance',
'chatgpt',
'claude',
'deepseek',
'discord',
'doubao',
'douyin',
'ernie',
'facebook',
'gemini',
'gmail',
'google',
'grok',
'huawei',
'icloud',
'instagram',
'jingdong',
'kakaotalk',
'kimi',
'kuaishou',
'line',
'meituan',
'messenger',
'meta',
'microsoft',
'netflix',
'openai',
'paypal',
'pinduoduo',
'qwen',
'reddit',
'signal',
'snapchat',
'taobao',
'telegram',
'tencent',
'threads',
'tiktok',
'tmall',
'tongyi',
'toutiao',
'twitter',
'viber',
'wechat',
'weibo',
'weixin',
'wenxin',
'whatsapp',
'xiaohongshu',
'xiaomi',
'youtube',
'zhihu'
) AND NOT (peer_type = 0 AND peer_id = 0) LIMIT 1);
CALL teamgram_fail_if_set(@bad, 'D-56: reserved username taken: ');

INSERT IGNORE INTO username (username, peer_type, peer_id, editable, active, order2, deleted) VALUES
('alibaba', 0, 0, 0, 0, 0, 0),
('alipay', 0, 0, 0, 0, 0, 0),
('aliyun', 0, 0, 0, 0, 0, 0),
('amazon', 0, 0, 0, 0, 0, 0),
('anthropic', 0, 0, 0, 0, 0, 0),
('apple', 0, 0, 0, 0, 0, 0),
('appstore', 0, 0, 0, 0, 0, 0),
('baidu', 0, 0, 0, 0, 0, 0),
('bilibili', 0, 0, 0, 0, 0, 0),
('botfather', 0, 0, 0, 0, 0, 0),
('bytedance', 0, 0, 0, 0, 0, 0),
('chatgpt', 0, 0, 0, 0, 0, 0),
('claude', 0, 0, 0, 0, 0, 0),
('deepseek', 0, 0, 0, 0, 0, 0),
('discord', 0, 0, 0, 0, 0, 0),
('doubao', 0, 0, 0, 0, 0, 0),
('douyin', 0, 0, 0, 0, 0, 0),
('ernie', 0, 0, 0, 0, 0, 0),
('facebook', 0, 0, 0, 0, 0, 0),
('gemini', 0, 0, 0, 0, 0, 0),
('gmail', 0, 0, 0, 0, 0, 0),
('google', 0, 0, 0, 0, 0, 0),
('grok', 0, 0, 0, 0, 0, 0),
('huawei', 0, 0, 0, 0, 0, 0),
('icloud', 0, 0, 0, 0, 0, 0),
('instagram', 0, 0, 0, 0, 0, 0),
('jingdong', 0, 0, 0, 0, 0, 0),
('kakaotalk', 0, 0, 0, 0, 0, 0),
('kimi', 0, 0, 0, 0, 0, 0),
('kuaishou', 0, 0, 0, 0, 0, 0),
('line', 0, 0, 0, 0, 0, 0),
('meituan', 0, 0, 0, 0, 0, 0),
('messenger', 0, 0, 0, 0, 0, 0),
('meta', 0, 0, 0, 0, 0, 0),
('microsoft', 0, 0, 0, 0, 0, 0),
('netflix', 0, 0, 0, 0, 0, 0),
('openai', 0, 0, 0, 0, 0, 0),
('paypal', 0, 0, 0, 0, 0, 0),
('pinduoduo', 0, 0, 0, 0, 0, 0),
('qwen', 0, 0, 0, 0, 0, 0),
('reddit', 0, 0, 0, 0, 0, 0),
('signal', 0, 0, 0, 0, 0, 0),
('snapchat', 0, 0, 0, 0, 0, 0),
('taobao', 0, 0, 0, 0, 0, 0),
('telegram', 0, 0, 0, 0, 0, 0),
('tencent', 0, 0, 0, 0, 0, 0),
('threads', 0, 0, 0, 0, 0, 0),
('tiktok', 0, 0, 0, 0, 0, 0),
('tmall', 0, 0, 0, 0, 0, 0),
('tongyi', 0, 0, 0, 0, 0, 0),
('toutiao', 0, 0, 0, 0, 0, 0),
('twitter', 0, 0, 0, 0, 0, 0),
('viber', 0, 0, 0, 0, 0, 0),
('wechat', 0, 0, 0, 0, 0, 0),
('weibo', 0, 0, 0, 0, 0, 0),
('weixin', 0, 0, 0, 0, 0, 0),
('wenxin', 0, 0, 0, 0, 0, 0),
('whatsapp', 0, 0, 0, 0, 0, 0),
('xiaohongshu', 0, 0, 0, 0, 0, 0),
('xiaomi', 0, 0, 0, 0, 0, 0),
('youtube', 0, 0, 0, 0, 0, 0),
('zhihu', 0, 0, 0, 0, 0, 0);

SET @teamgram_stmt = 'UPDATE username SET peer_type = 0, peer_id = 0, editable = 0, active = 0, order2 = 0, deleted = 0 WHERE peer_type = 0 AND peer_id = 0 AND username IN (''alibaba'', ''alipay'', ''aliyun'', ''amazon'', ''anthropic'', ''apple'', ''appstore'', ''baidu'', ''bilibili'', ''botfather'', ''bytedance'', ''chatgpt'', ''claude'', ''deepseek'', ''discord'', ''doubao'', ''douyin'', ''ernie'', ''facebook'', ''gemini'', ''gmail'', ''google'', ''grok'', ''huawei'', ''icloud'', ''instagram'', ''jingdong'', ''kakaotalk'', ''kimi'', ''kuaishou'', ''line'', ''meituan'', ''messenger'', ''meta'', ''microsoft'', ''netflix'', ''openai'', ''paypal'', ''pinduoduo'', ''qwen'', ''reddit'', ''signal'', ''snapchat'', ''taobao'', ''telegram'', ''tencent'', ''threads'', ''tiktok'', ''tmall'', ''tongyi'', ''toutiao'', ''twitter'', ''viber'', ''wechat'', ''weibo'', ''weixin'', ''wenxin'', ''whatsapp'', ''xiaohongshu'', ''xiaomi'', ''youtube'', ''zhihu'')';
PREPARE teamgram_ps FROM @teamgram_stmt;
EXECUTE teamgram_ps;
DEALLOCATE PREPARE teamgram_ps;
