import React from 'react';
import { useTranslation } from 'react-i18next';
import { Receipt, ShieldCheck, Lock, Users } from 'lucide-react';
import SectionHeader from '../components/SectionHeader';

const trustModules = [
  {
    key: 'compliance',
    icon: Receipt,
    color: 'blue',
    bgLight: 'bg-blue-50',
    bgDark: 'dark:bg-blue-900/30',
    textLight: 'text-blue-600',
    textDark: 'dark:text-blue-400',
  },
  {
    key: 'stability',
    icon: ShieldCheck,
    color: 'emerald',
    bgLight: 'bg-emerald-50',
    bgDark: 'dark:bg-emerald-900/30',
    textLight: 'text-emerald-600',
    textDark: 'dark:text-emerald-400',
    stat: '99.9%',
  },
  {
    key: 'security',
    icon: Lock,
    color: 'purple',
    bgLight: 'bg-purple-50',
    bgDark: 'dark:bg-purple-900/30',
    textLight: 'text-purple-600',
    textDark: 'dark:text-purple-400',
  },
  {
    key: 'team',
    icon: Users,
    color: 'amber',
    bgLight: 'bg-amber-50',
    bgDark: 'dark:bg-amber-900/30',
    textLight: 'text-amber-600',
    textDark: 'dark:text-amber-400',
  },
];

const TrustSection = ({ isMobile }) => {
  const { t } = useTranslation();

  return (
    <section className='w-full'>
      <div className='home-section'>
        <SectionHeader
          title={t('为您筑起 AI 应用的安全长城')}
        />

        <div className='grid grid-cols-1 md:grid-cols-2 gap-4 md:gap-6'>
          {trustModules.map((mod, idx) => {
            const Icon = mod.icon;
            return (
              <div
                key={mod.key}
                className={`home-card home-fade-in-up home-delay-${idx + 1} flex items-start gap-4 group`}
              >
                {/* Icon */}
                <div
                  className={`flex-shrink-0 w-12 h-12 rounded-xl ${mod.bgLight} ${mod.bgDark} flex items-center justify-center`}
                >
                  <Icon
                    className={`w-6 h-6 ${mod.textLight} ${mod.textDark}`}
                  />
                </div>

                {/* Content */}
                <div className='flex-1 min-w-0'>
                  <div className='flex items-center gap-2 mb-1'>
                    <h3 className='text-base font-semibold text-semi-color-text-0'>
                      {t(`trust_${mod.key}_title`)}
                    </h3>
                    {mod.stat && (
                      <span className={`text-xs font-bold px-2 py-0.5 rounded-full ${mod.bgLight} ${mod.bgDark} ${mod.textLight} ${mod.textDark}`}>
                        {mod.stat}
                      </span>
                    )}
                  </div>
                  <p className='text-sm text-semi-color-text-2 leading-relaxed'>
                    {t(`trust_${mod.key}_desc`)}
                  </p>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
};

export default TrustSection;
