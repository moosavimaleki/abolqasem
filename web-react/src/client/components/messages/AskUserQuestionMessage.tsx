import { useState } from "react"
import { ArrowLeft, Check, MessageCircleQuestion, X } from "lucide-react"
import type { ProcessedToolCall, AskUserQuestionItem, AskUserQuestionOption } from "./types"
import type { AskUserQuestionAnswerMap } from "../../../shared/types"
import { Button } from "../ui/button"
import { cn } from "../../lib/utils"
import { useTranscriptRenderOptions } from "./render-context"
import { useI18n } from "../../i18n/context"

interface Props {
  message: Extract<ProcessedToolCall, { toolKind: "ask_user_question" }>
  onSubmit: (toolUseId: string, questions: AskUserQuestionItem[], answers: AskUserQuestionAnswerMap) => void
  isLatest: boolean
}

// Local components for DRY

function QuestionCard({
  header,
  question,
  currentIndex,
  totalQuestions,
  onBack,
  children
}: {
  header?: string
  question: string
  currentIndex: number
  totalQuestions: number
  onBack?: () => void
  children: React.ReactNode
}) {
  const showBackButton = onBack && currentIndex > 0

  return (
    <section className="overflow-hidden rounded-2xl border border-border/80 bg-card/60 shadow-sm">
      <div className="relative border-b border-border/70 bg-muted/25 px-4 py-3.5">
        <div className="flex items-start gap-3">
          {showBackButton ? (
            <button
              type="button"
              onClick={onBack}
              aria-label="Back to previous question"
              className="-ms-1 mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <ArrowLeft className="size-4 rtl:rotate-180" />
            </button>
          ) : null}
          <div className="min-w-0 flex-1">
            <div className="mb-1.5 flex items-center justify-between gap-3">
              {header ? (
                <span className="rounded-md bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground">
                  {header}
                </span>
              ) : <span />}
              {totalQuestions > 1 ? (
                <span className="shrink-0 text-xs tabular-nums text-muted-foreground">{currentIndex + 1} / {totalQuestions}</span>
              ) : null}
            </div>
            <h3 className="text-pretty text-[15px] font-semibold leading-6 text-foreground">{question}</h3>
          </div>
        </div>
        {totalQuestions > 1 && (
          <div className="absolute inset-x-0 bottom-0 h-px bg-border/60">
            <div
              className="h-full bg-primary/70 transition-[width] duration-200 motion-reduce:transition-none"
              style={{ width: `${((currentIndex + 1) / totalQuestions) * 100}%` }}
            />
          </div>
        )}
      </div>
      {children}
    </section>
  )
}

function OptionContent({ label, description }: { label: string; description?: string }) {
  return (
    <div className="min-w-0 flex-1">
      <span className="block text-sm font-medium leading-5 text-foreground">{label}</span>
      {description && (
        <p className="mt-1 text-pretty text-xs leading-5 text-muted-foreground">{description}</p>
      )}
    </div>
  )
}

function SelectionIndicator({
  selected,
  multiSelect,
}: {
  selected: boolean
  multiSelect?: boolean
}) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "flex size-5 shrink-0 items-center justify-center border transition-colors duration-150",
        multiSelect ? "rounded" : "rounded-full",
        selected
          ? "border-primary bg-primary text-primary-foreground"
          : "border-muted-foreground/45 bg-background text-transparent"
      )}
    >
      <Check strokeWidth={3} className="size-3" />
    </span>
  )
}

function OptionRow({
  option,
  selected,
  multiSelect,
  onClick,
}: {
  option: AskUserQuestionOption
  selected: boolean
  multiSelect?: boolean
  onClick?: () => void
}) {
  if (onClick) {
    return (
      <button
        type="button"
        onClick={onClick}
        role={multiSelect ? "checkbox" : "radio"}
        aria-checked={selected}
        className={cn(
          "group flex min-h-14 w-full cursor-pointer items-center gap-3 rounded-xl border px-3.5 py-3 text-start transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background",
          selected
            ? "border-primary/55 bg-primary/10"
            : "border-border/75 bg-background/55 hover:border-border hover:bg-muted/35",
        )}
      >
        <SelectionIndicator selected={selected} multiSelect={multiSelect} />
        <OptionContent label={option.label} description={option.description} />
      </button>
    )
  }

  return (
    <div className="rounded-xl border border-border/70 bg-background/55 px-3.5 py-3">
      <OptionContent label={option.label} description={option.description} />
    </div>
  )
}

function parseAnswersFromResult(
  result: Extract<ProcessedToolCall, { toolKind: "ask_user_question" }>["result"]
): AskUserQuestionAnswerMap | undefined {
  return result?.answers
}

function getQuestionKey(question: AskUserQuestionItem): string {
  return question.id || question.question
}

export function AskUserQuestionMessage({ message, onSubmit, isLatest }: Props) {
  const { t } = useI18n()
  const renderOptions = useTranscriptRenderOptions()
  const questions = message.input.questions
  const isComplete = !!message.result
  const savedAnswers = parseAnswersFromResult(message.result)
  const isDiscarded = message.result?.discarded === true

  const [currentIndex, setCurrentIndex] = useState(0)
  const [answers, setAnswers] = useState<Record<string, string>>({})
  const [customInputs, setCustomInputs] = useState<Record<string, string>>({})
  const [submittedAnswers, setSubmittedAnswers] = useState<AskUserQuestionAnswerMap | null>(savedAnswers ?? null)
  const [isSubmitted, setIsSubmitted] = useState(isComplete)

  const getEffectiveAnswers = (questionKey: string, question?: AskUserQuestionItem) => {
    const custom = customInputs[questionKey]?.trim()
    const selectedAnswer = answers[questionKey] || ""
    const q = question || questions.find((candidate) => getQuestionKey(candidate) === questionKey)

    if (q?.multiSelect) {
      return [selectedAnswer, custom]
        .filter(Boolean)
        .flatMap((value) => value.split(", ").filter(Boolean))
    }

    const value = custom || selectedAnswer
    return value ? [value] : []
  }

  const getSelectedOptions = (question: AskUserQuestionItem) => {
    const answer = answers[getQuestionKey(question)] || ""
    return question.multiSelect
      ? answer.split(", ").filter(Boolean)
      : [answer]
  }

  const handleOptionSelect = (question: AskUserQuestionItem, label: string) => {
    const key = getQuestionKey(question)

    if (question.multiSelect) {
      const current = answers[key] ? answers[key].split(", ").filter(Boolean) : []
      const newSelection = current.includes(label)
        ? current.filter((o) => o !== label)
        : [...current, label]
      setAnswers({ ...answers, [key]: newSelection.join(", ") })
    } else {
      setAnswers({ ...answers, [key]: label })
      setCustomInputs({ ...customInputs, [key]: "" })
      // Auto-advance to next question for single select
      if (currentIndex < questions.length - 1) {
        setTimeout(() => setCurrentIndex(currentIndex + 1), 150)
      }
    }
  }

  const handleCustomInputChange = (question: AskUserQuestionItem, value: string) => {
    const key = getQuestionKey(question)
    setCustomInputs({ ...customInputs, [key]: value })
    if (value && !question.multiSelect) {
      setAnswers({ ...answers, [key]: "" })
    }
  }

  const clearCustomInput = (question: AskUserQuestionItem) => {
    const key = getQuestionKey(question)
    if (question.multiSelect && customInputs[key]) {
      setCustomInputs({ ...customInputs, [key]: "" })
    }
  }

  const allQuestionsAnswered = questions.every((question) => getEffectiveAnswers(getQuestionKey(question), question).length > 0)
  const currentQuestion = questions[currentIndex]
  const isLastQuestion = currentIndex === questions.length - 1
  const currentHasAnswer = currentQuestion
    && getEffectiveAnswers(getQuestionKey(currentQuestion), currentQuestion).length > 0

  const handleNext = () => {
    if (currentIndex < questions.length - 1) {
      setCurrentIndex(currentIndex + 1)
    }
  }

  const handleBack = () => {
    if (currentIndex > 0) {
      setCurrentIndex(currentIndex - 1)
    }
  }

  const handleSubmit = () => {
    if (!allQuestionsAnswered) return

    const finalAnswers: AskUserQuestionAnswerMap = {}
    for (const q of questions) {
      const key = getQuestionKey(q)
      finalAnswers[key] = getEffectiveAnswers(key, q)
    }
    setSubmittedAnswers(finalAnswers)
    setIsSubmitted(true)
    onSubmit(message.toolId, questions, finalAnswers)
  }

  const handleCustomInputEnter = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key !== "Enter") return
    if (!currentQuestion || !currentHasAnswer) return

    event.preventDefault()

    if (isLastQuestion) {
      handleSubmit()
      return
    }

    handleNext()
  }

  // Completed state
  if (isSubmitted || isComplete) {
    const displayAnswers = savedAnswers || submittedAnswers || {}

    return (
      <div className="mx-auto w-full max-w-2xl">
        <div className="overflow-hidden rounded-2xl border border-border/80 bg-card/60 shadow-sm">
          <div className="flex items-center justify-between border-b border-border/70 bg-muted/25 px-4 py-3 text-sm font-medium">
            <p>{questions.length !== 1 ? t.messages.questions : t.messages.question}</p>
            <p className="">{isDiscarded ? t.messages.discarded : t.messages.answers}</p>
          </div>
          {questions.map((question, index) => {
            const answerValue = displayAnswers[getQuestionKey(question)] || displayAnswers[question.question] || []
            const isLast = index === questions.length - 1

            return (
              <div
                key={getQuestionKey(question)}
                className={cn(
                  "flex w-full items-center justify-between gap-4 bg-background/45 px-4 py-3",
                  !isLast && "border-b border-border"
                )}
              >
                <div className="text-sm text-pretty">{question.question}</div>
                {answerValue.length > 0 && <div className="text-sm font-medium text-right max-w-[50%] text-pretty">{answerValue.join(", ")}</div>}
                {answerValue.length === 0 && (
                  <div className="text-sm font-medium text-right italic">
                    {isDiscarded ? t.messages.discarded : t.messages.noResponse}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      </div>
    )
  }

  if (renderOptions.readonly) {
    return (
      <div className="mx-auto w-full max-w-2xl">
        <div className="overflow-hidden rounded-2xl border border-border/80 bg-card/60 shadow-sm">
          <div className="flex items-center justify-between gap-3 border-b border-border/70 bg-muted/25 px-4 py-3 text-sm font-medium">
            <p>{questions.length !== 1 ? t.messages.questions : t.messages.question}</p>
            <p className="text-muted-foreground">{t.messages.awaitingResponse}</p>
          </div>
          {questions.map((question, index) => (
            <div
              key={getQuestionKey(question)}
              className={cn(
                "flex w-full items-center justify-between gap-4 bg-background/45 px-4 py-3",
                index < questions.length - 1 && "border-b border-border",
              )}
            >
              <div className="text-sm text-pretty">{question.question}</div>
              <div className="max-w-[50%] text-right text-xs text-muted-foreground text-pretty">
                {question.options?.map((option) => option.label).join(", ") || t.messages.freeformResponse}
              </div>
            </div>
          ))}
        </div>
      </div>
    )
  }

  // Pending state (not latest)
  if (!isLatest) {
    return (
      <div className="w-full py-2">
        <div className="flex items-center gap-2">
          <MessageCircleQuestion className="h-4 w-4 text-muted-foreground" />
          <span className="text-sm text-muted-foreground">{t.messages.questionsPending}</span>
        </div>
      </div>
    )
  }

  // Active state - show one question at a time
  if (!currentQuestion) return null

  const selectedOptions = getSelectedOptions(currentQuestion)
  const customInput = customInputs[getQuestionKey(currentQuestion)] || ""

  return (
    <div className="mx-auto w-full max-w-2xl">
      <QuestionCard
        header={currentQuestion.header}
        question={currentQuestion.question}
        currentIndex={currentIndex}
        totalQuestions={questions.length}
        onBack={currentIndex > 0 ? handleBack : undefined}
      >
        <div className="space-y-2 p-3">
          <div
            className="space-y-2"
            role={currentQuestion.multiSelect ? "group" : "radiogroup"}
            aria-label={currentQuestion.question}
          >
            {currentQuestion.options?.map((option) => (
              <OptionRow
                key={option.label}
                option={option}
                selected={selectedOptions.includes(option.label)}
                multiSelect={currentQuestion.multiSelect}
                onClick={() => handleOptionSelect(currentQuestion, option.label)}
              />
            ))}
          </div>

          <div className={cn(
            "flex min-h-14 items-center gap-3 rounded-xl border bg-background/55 px-3.5 transition-colors duration-150 focus-within:ring-2 focus-within:ring-ring focus-within:ring-offset-2 focus-within:ring-offset-background",
            customInput ? "border-primary/55 bg-primary/10" : "border-border/75",
          )}>
            <SelectionIndicator selected={!!customInput} multiSelect={currentQuestion.multiSelect} />
            <input
              type="text"
              value={customInput}
              onChange={(e) => handleCustomInputChange(currentQuestion, e.target.value)}
              onKeyDown={handleCustomInputEnter}
              placeholder={t.messages.other}
              aria-label={t.messages.other}
              className="min-w-0 flex-1 bg-transparent py-3 text-sm text-foreground outline-none placeholder:text-muted-foreground"
            />
            {currentQuestion.multiSelect && customInput ? (
              <button
                type="button"
                onClick={() => clearCustomInput(currentQuestion)}
                className="rounded-md px-2 py-1 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <X className="size-3.5" />
              </button>
            ) : null}
          </div>
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-border/70 bg-muted/15 px-3 py-2.5">
          {!isLastQuestion && currentHasAnswer && (currentQuestion.multiSelect || !!customInput) ? (
            <Button size="sm" onClick={handleNext} className="min-w-20 gap-1.5">
              {t.common.next}
              <ArrowLeft className="size-3.5 rtl:rotate-180" />
            </Button>
          ) : null}
          {isLastQuestion ? (
            <Button
              size="sm"
              onClick={handleSubmit}
              disabled={!allQuestionsAnswered}
              className="min-w-20"
            >
              {t.common.submit}
            </Button>
          ) : null}
        </div>
      </QuestionCard>
    </div>
  )
}
