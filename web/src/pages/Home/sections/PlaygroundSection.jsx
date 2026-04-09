import React, { useState, useEffect, useRef, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@douyinfe/semi-ui';
import { IconCopy } from '@douyinfe/semi-icons';
import { copy, showSuccess } from '../../../helpers';
import SectionHeader from '../components/SectionHeader';

const codeExamples = {
  curl: `curl https://api.example.com/v1/chat/completions \\
  -H "Authorization: Bearer sk-xxxxxxxxx" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-4o",
    "messages": [
      {"role": "user", "content": "Hello!"}
    ]
  }'`,
  python: `from openai import OpenAI

client = OpenAI(
    api_key="sk-xxxxxxxxx",
    base_url="https://api.example.com/v1"
)

response = client.chat.completions.create(
    model="gpt-4o",
    messages=[
        {"role": "user", "content": "Hello!"}
    ]
)
print(response.choices[0].message.content)`,
  nodejs: `import OpenAI from 'openai';

const client = new OpenAI({
  apiKey: 'sk-xxxxxxxxx',
  baseURL: 'https://api.example.com/v1',
});

const response = await client.chat.completions.create({
  model: 'gpt-4o',
  messages: [{ role: 'user', content: 'Hello!' }],
});
console.log(response.choices[0].message.content);`,
};

const responseJson = `{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "model": "gpt-4o",
  "choices": [{
    "index": 0,
    "message": {
      "role": "assistant",
      "content": "Hello! How can I help you today?"
    },
    "finish_reason": "stop"
  }],
  "usage": {
    "prompt_tokens": 9,
    "completion_tokens": 12,
    "total_tokens": 21
  }
}`;

const tabs = [
  { key: 'curl', label: 'cURL' },
  { key: 'python', label: 'Python' },
  { key: 'nodejs', label: 'Node.js' },
];

const PlaygroundSection = ({ isMobile }) => {
  const { t } = useTranslation();
  const [activeTab, setActiveTab] = useState('python');
  const [visibleChars, setVisibleChars] = useState(0);
  const intervalRef = useRef(null);
  const isPlayingRef = useRef(true);

  const handleCopyCode = async () => {
    const ok = await copy(codeExamples[activeTab]);
    if (ok) {
      showSuccess(t('已复制到剪切板'));
    }
  };

  const resetAnimation = useCallback(() => {
    setVisibleChars(0);
    isPlayingRef.current = true;
  }, []);

  useEffect(() => {
    resetAnimation();
  }, [activeTab, resetAnimation]);

  useEffect(() => {
    intervalRef.current = setInterval(() => {
      if (document.hidden || !isPlayingRef.current) return;
      setVisibleChars((prev) => {
        if (prev >= responseJson.length) {
          isPlayingRef.current = false;
          return prev;
        }
        return prev + 1;
      });
    }, 20);
    return () => clearInterval(intervalRef.current);
  }, []);

  const activeCode = codeExamples[activeTab];
  const displayedResponse = responseJson.slice(0, visibleChars);
  const isTyping = visibleChars < responseJson.length;

  return (
    <section className='w-full home-section-alt'>
      <div className='home-section'>
        <SectionHeader
          title={t('几行代码，即刻接入')}
          subtitle={t('兼容 OpenAI SDK，无需修改任何代码，只需替换 base_url 即可使用')}
        />

        <div className={`flex ${isMobile ? 'flex-col' : 'flex-row'} gap-4 md:gap-6`}>
          {/* Code block (left) */}
          <div className={`${isMobile ? 'w-full' : 'w-3/5'} home-code-block`}>
            {/* Tab bar */}
            <div className='flex items-center justify-between border-b border-gray-700/50 px-4'>
              <div className='flex'>
                {tabs.map((tab) => (
                  <button
                    key={tab.key}
                    onClick={() => setActiveTab(tab.key)}
                    className={`px-4 py-2.5 text-sm font-medium transition-colors ${
                      activeTab === tab.key
                        ? 'text-white border-b-2 border-blue-400'
                        : 'text-gray-400 hover:text-gray-200'
                    }`}
                  >
                    {tab.label}
                  </button>
                ))}
              </div>
              <button
                onClick={handleCopyCode}
                className='text-gray-400 hover:text-white transition-colors p-1.5 rounded-md hover:bg-gray-700/50'
                title={t('复制代码')}
              >
                <IconCopy size={16} />
              </button>
            </div>

            {/* Code content */}
            <div className='p-4 overflow-x-auto'>
              <pre className='text-sm text-gray-300 font-mono leading-relaxed whitespace-pre'>
                {activeCode}
              </pre>
            </div>
          </div>

          {/* Response preview (right) */}
          <div className={`${isMobile ? 'w-full' : 'w-2/5'}`}>
            <div className='home-card h-full flex flex-col'>
              {/* Status bar */}
              <div className='flex items-center gap-2 mb-3 pb-3 border-b border-semi-color-border'>
                <div className='w-2 h-2 rounded-full bg-green-500' />
                <span className='text-xs font-mono text-semi-color-text-2'>
                  200 OK
                </span>
                <span className='text-xs text-semi-color-text-3 ml-auto'>
                  {t('响应预览')}
                </span>
              </div>

              {/* JSON content */}
              <div className='flex-1 overflow-auto'>
                <pre className='text-sm font-mono leading-relaxed whitespace-pre-wrap break-all'>
                  <span className='text-semi-color-text-1'>
                    {isTyping ? (
                      <span className='home-typing-cursor'>{displayedResponse}</span>
                    ) : (
                      displayedResponse
                    )}
                  </span>
                </pre>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
};

export default PlaygroundSection;
