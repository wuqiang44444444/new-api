import React from 'react';
import { useTranslation } from 'react-i18next';
import { Zap, Palette, Bot, Target } from 'lucide-react';
import SectionHeader from '../components/SectionHeader';

const capabilities = [
  {
    key: 'stable_llm',
    icon: Zap,
    color: 'blue',
    bgLight: 'bg-blue-50',
    bgDark: 'dark:bg-blue-900/30',
    textLight: 'text-blue-600',
    textDark: 'dark:text-blue-400',
    borderLight: 'group-hover:border-blue-200',
    borderDark: 'dark:group-hover:border-blue-800',
  },
  {
    key: 'visual',
    icon: Palette,
    color: 'purple',
    bgLight: 'bg-purple-50',
    bgDark: 'dark:bg-purple-900/30',
    textLight: 'text-purple-600',
    textDark: 'dark:text-purple-400',
    borderLight: 'group-hover:border-purple-200',
    borderDark: 'dark:group-hover:border-purple-800',
  },
  {
    key: 'agent',
    icon: Bot,
    color: 'emerald',
    bgLight: 'bg-emerald-50',
    bgDark: 'dark:bg-emerald-900/30',
    textLight: 'text-emerald-600',
    textDark: 'dark:text-emerald-400',
    borderLight: 'group-hover:border-emerald-200',
    borderDark: 'dark:group-hover:border-emerald-800',
  },
  {
    key: 'scenario',
    icon: Target,
    color: 'amber',
    bgLight: 'bg-amber-50',
    bgDark: 'dark:bg-amber-900/30',
    textLight: 'text-amber-600',
    textDark: 'dark:text-amber-400',
    borderLight: 'group-hover:border-amber-200',
    borderDark: 'dark:group-hover:border-amber-800',
  },
];

const CapabilitySection = ({ isMobile }) => {
  const { t } = useTranslation();

  return (
    <section className='w-full home-section-alt'>
      <div className='home-section'>
        <SectionHeader
          title={t('超越模型，更是端到端的赋能')}
          subtitle={t('按业务需求匹配最优算力与开发套件，让每一分投入都有高产出')}
        />

        <div className='grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 md:gap-6'>
          {capabilities.map((cap, idx) => {
            const Icon = cap.icon;
            return (
              <div
                key={cap.key}
                className={`home-card home-fade-in-up home-delay-${idx + 1} group cursor-default ${cap.borderLight} ${cap.borderDark}`}
              >
                {/* Icon badge */}
                <div
                  className={`w-12 h-12 rounded-xl ${cap.bgLight} ${cap.bgDark} flex items-center justify-center mb-4`}
                >
                  <Icon
                    className={`w-6 h-6 ${cap.textLight} ${cap.textDark}`}
                  />
                </div>

                {/* Title */}
                <h3 className='text-lg font-semibold text-semi-color-text-0 mb-2'>
                  {t(`capability_${cap.key}_title`)}
                </h3>

                {/* Description */}
                <p className='text-sm text-semi-color-text-2 mb-3 leading-relaxed'>
                  {t(`capability_${cap.key}_desc`)}
                </p>

                {/* Tags */}
                <div className='flex flex-wrap gap-1.5'>
                  {[1, 2, 3].map((tagIdx) => (
                    <span
                      key={tagIdx}
                      className={`text-xs px-2 py-0.5 rounded-full ${cap.bgLight} ${cap.bgDark} ${cap.textLight} ${cap.textDark}`}
                    >
                      {t(`capability_${cap.key}_tag${tagIdx}`)}
                    </span>
                  ))}
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
};

export default CapabilitySection;
