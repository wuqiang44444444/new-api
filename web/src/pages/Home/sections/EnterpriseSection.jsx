import React from 'react';
import { useTranslation } from 'react-i18next';
import { Headphones, Plug, Server } from 'lucide-react';
import SectionHeader from '../components/SectionHeader';

const enterpriseCards = [
  {
    key: 'ai_service',
    icon: Headphones,
    color: 'blue',
    bgLight: 'bg-blue-50',
    bgDark: 'dark:bg-blue-900/30',
    textLight: 'text-blue-600',
    textDark: 'dark:text-blue-400',
    borderLight: 'group-hover:border-blue-200',
    borderDark: 'dark:group-hover:border-blue-800',
  },
  {
    key: 'integration',
    icon: Plug,
    color: 'emerald',
    bgLight: 'bg-emerald-50',
    bgDark: 'dark:bg-emerald-900/30',
    textLight: 'text-emerald-600',
    textDark: 'dark:text-emerald-400',
    borderLight: 'group-hover:border-emerald-200',
    borderDark: 'dark:group-hover:border-emerald-800',
  },
  {
    key: 'private_deploy',
    icon: Server,
    color: 'purple',
    bgLight: 'bg-purple-50',
    bgDark: 'dark:bg-purple-900/30',
    textLight: 'text-purple-600',
    textDark: 'dark:text-purple-400',
    borderLight: 'group-hover:border-purple-200',
    borderDark: 'dark:group-hover:border-purple-800',
  },
];

const EnterpriseSection = ({ isMobile }) => {
  const { t } = useTranslation();

  return (
    <section className='w-full'>
      <div className='home-section'>
        <SectionHeader
          title={t('不只是算力供应商，更是您的全栈 AI 落地伙伴')}
        />

        <div className='grid grid-cols-1 md:grid-cols-3 gap-4 md:gap-6'>
          {enterpriseCards.map((card, idx) => {
            const Icon = card.icon;
            return (
              <div
                key={card.key}
                className={`home-card home-fade-in-up home-delay-${idx + 1} group ${card.borderLight} ${card.borderDark}`}
              >
                {/* Icon */}
                <div
                  className={`w-12 h-12 rounded-xl ${card.bgLight} ${card.bgDark} flex items-center justify-center mb-4`}
                >
                  <Icon
                    className={`w-6 h-6 ${card.textLight} ${card.textDark}`}
                  />
                </div>

                {/* Title */}
                <h3 className='text-lg font-semibold text-semi-color-text-0 mb-2'>
                  {t(`enterprise_${card.key}_title`)}
                </h3>

                {/* Description */}
                <p className='text-sm text-semi-color-text-2 leading-relaxed'>
                  {t(`enterprise_${card.key}_desc`)}
                </p>
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
};

export default EnterpriseSection;
