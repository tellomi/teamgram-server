// Copyright (c) 2026 The Teamgram Authors (https://teamgram.net).
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)

package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/richmsg"
)

// MessagesComposeRichMessageWithAI
// messages.composeRichMessageWithAI#8d7ae6af flags:# proofread:flags.0?true emojify:flags.3?true text:flags.4?InputRichMessage translate_to_lang:flags.1?string tone:flags.2?InputAiComposeTone = messages.ComposedRichMessageWithAI;
// https://core.telegram.org/method/messages.composeMessageWithAI
func (c *MessagesCore) MessagesComposeRichMessageWithAI(in *mtproto.TLMessagesComposeRichMessageWithAI) (*mtproto.Messages_ComposedRichMessageWithAI, error) {
	return richmsg.Compose(in)
}
