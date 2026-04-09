import React from 'react';
import { Button, Typography } from '@douyinfe/semi-ui';
import {
  IconGithubLogo,
  IconPlay,
  IconFile,
} from '@douyinfe/semi-icons';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import EndpointTicker from '../components/EndpointTicker';
import {
  Moonshot,
  OpenAI,
  XAI,
  Zhipu,
  Volcengine,
  Cohere,
  Claude,
  Gemini,
  Suno,
  Minimax,
  Wenxin,
  Spark,
  Qingyan,
  DeepSeek,
  Qwen,
  Midjourney,
  Grok,
  AzureAI,
  Hunyuan,
  Xinference,
} from '@lobehub/icons';

const { Text } = Typography;

const providerIcons = [
  { Component: Moonshot, color: false },
  { Component: OpenAI, color: false },
  { Component: XAI, color: false },
  { Component: Zhipu, color: true },
  { Component: Volcengine, color: true },
  { Component: Cohere, color: true },
  { Component: Claude, color: true },
  { Component: Gemini, color: true },
  { Component: Suno, color: false },
  { Component: Minimax, color: true },
  { Component: Wenxin, color: true },
  { Component: Spark, color: true },
  { Component: Qingyan, color: true },
  { Component: DeepSeek, color: true },
  { Component: Qwen, color: true },
  { Component: Midjourney, color: false },
  { Component: Grok, color: false },
  { Component: AzureAI, color: true },
  { Component: Hunyuan, color: true },
  { Component: Xinference, color: true },
];

const HeroSection = ({
  serverAddress,
  endpointItems,
  isMobile,
  isDemoSiteMode,
  docsLink,
  statusVersion,
}) => {
  const { t, i18n } = useTranslation();
  const isChinese = i18n.language.startsWith('zh');

  return (
    <div className='w-full border-b border-semi-color-border min-h-[500px] md:min-h-[600px] lg:min-h-[700px] relative overflow-x-hidden'>
      {/* Background blur blobs with float animation */}
      <div className='blur-ball blur-ball-indigo home-hero-blob' />
      <div className='blur-ball blur-ball-teal home-hero-blob-delayed' />

      <div className='flex items-center justify-center h-full px-4 py-20 md:py-24 lg:py-32 mt-10'>
        <div className='flex flex-col items-center justify-center text-center max-w-4xl mx-auto'>
          {/* Title */}
          <div className='flex flex-col items-center justify-center mb-6 md:mb-8'>
            <h1
              className={`text-4xl md:text-5xl lg:text-6xl xl:text-7xl font-bold text-semi-color-text-0 leading-tight ${isChinese ? 'tracking-wide md:tracking-wider' : ''}`}
            >
              <>
                {t('统一的')}
                <br />
                <span className='shine-text'>{t('大模型接口网关')}</span>
              </>
            </h1>
            <p className='text-base md:text-lg lg:text-xl text-semi-color-text-1 mt-4 md:mt-6 max-w-xl'>
              {t('更好的价格，更好的稳定性，只需要将模型基址替换为：')}
            </p>

            {/* Endpoint ticker */}
            <div className='mt-4 md:mt-6'>
              <EndpointTicker
                serverAddress={serverAddress}
                endpointItems={endpointItems}
                isMobile={isMobile}
              />
            </div>
          </div>

          {/* CTA Buttons */}
          <div className='flex flex-row gap-4 justify-center items-center'>
            <Link to='/console'>
              <Button
                theme='solid'
                type='primary'
                size={isMobile ? 'default' : 'large'}
                className='!rounded-3xl px-8 py-2'
                icon={<IconPlay />}
              >
                {t('获取密钥')}
              </Button>
            </Link>
            {isDemoSiteMode && statusVersion ? (
              <Button
                size={isMobile ? 'default' : 'large'}
                className='flex items-center !rounded-3xl px-6 py-2'
                icon={<IconGithubLogo />}
                onClick={() =>
                  window.open(
                    'https://github.com/QuantumNous/new-api',
                    '_blank',
                  )
                }
              >
                {statusVersion}
              </Button>
            ) : (
              docsLink && (
                <Button
                  size={isMobile ? 'default' : 'large'}
                  className='flex items-center !rounded-3xl px-6 py-2'
                  icon={<IconFile />}
                  onClick={() => window.open(docsLink, '_blank')}
                >
                  {t('文档')}
                </Button>
              )
            )}
          </div>

          {/* Provider Icons */}
          <div className='mt-12 md:mt-16 lg:mt-20 w-full'>
            <div className='flex items-center mb-6 md:mb-8 justify-center'>
              <Text
                type='tertiary'
                className='text-lg md:text-xl lg:text-2xl font-light'
              >
                {t('支持众多的大模型供应商')}
              </Text>
            </div>
            <div className='flex flex-wrap items-center justify-center gap-3 sm:gap-4 md:gap-6 lg:gap-8 max-w-5xl mx-auto px-4'>
              {providerIcons.map(({ Component, color }, idx) => (
                <div
                  key={idx}
                  className='w-8 h-8 sm:w-10 sm:h-10 md:w-12 md:h-12 flex items-center justify-center'
                >
                  {color ? (
                    <Component.Color size={40} />
                  ) : (
                    <Component size={40} />
                  )}
                </div>
              ))}
              <div className='w-8 h-8 sm:w-10 sm:h-10 md:w-12 md:h-12 flex items-center justify-center'>
                <Typography.Text className='!text-lg sm:!text-xl md:!text-2xl lg:!text-3xl font-bold'>
                  30+
                </Typography.Text>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default HeroSection;
