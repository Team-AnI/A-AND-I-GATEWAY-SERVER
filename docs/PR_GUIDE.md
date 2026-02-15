# Code Style & PR Guide

**'우아한 테크코스' 클린 코드 원칙**을 지향합니다.

모든 PR은 아래 규칙을 준수해야 Merge될 수 있습니다.

## 1. Code Constraints (엄격 준수)

1. **Indent(들여쓰기) Depth는 1까지만 허용한다.**

   - `if`, `for`, `while` 등이 중첩되면 메서드로 분리한다.

2. **`else` 예약어를 사용하지 않는다.**

   - Early Return 패턴을 사용하여 가독성을 높인다.

3. **모든 원시값(Primitive)과 문자열을 포장(Wrapping)한다.**

   - 예: `int age` -> `class Age`

4. **일급 컬렉션(First Class Collection)을 사용한다.**

   - `List<Member>` 등의 컬렉션을 필드로 가지는 별도 클래스를 만든다.

5. **3개 이상의 인스턴스 변수를 가진 클래스를 쓰지 않는다.**

6. **Getter/Setter 사용을 지양한다.**

   - 객체에서 데이터를 꺼내지 말고, 객체에 메시지를 보내라. (DTO는 예외)

7. **한 메서드는 오직 한 가지 일만 해야 한다.**

## 2. Style Guide

- **Kotlin/Java:** [Google Java Style Guide](https://google.github.io/styleguide/javaguide.html)를 따른다.

## 3. PR Template

PR 작성 시 다음 양식을 따른다:

## Title

[Type]: [Subject]

## Description

- 무엇을(What), 왜(Why) 변경했는지 설명
- 관련 이슈 링크 (Closes #Issue)

## Key Code (Before & After)

- 핵심 로직의 변경 전/후 비교

## Reason for Change

- 기술적 배경 및 개선 이유

## To Reviewer

- 리뷰어가 중점적으로 봐주었으면 하는 부분


